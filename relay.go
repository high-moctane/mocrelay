package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

const (
	RateLimitRate  = 20
	RateLimitBurst = 10
	MaxFilterLen   = 50
)

var DefaultRelay = NewRelay()

func NewRelay() *Relay {
	return &Relay{
		router: NewRouter(DefaultFilters, *MaxReqSubIDNum),
		cache:  NewCache(*CacheSize, DefaultFilters),
	}
}

type Relay struct {
	router *Router
	cache  *Cache
}

func (relay *Relay) NewHandler(req *http.Request, connID string) *RelayHandler {
	chanBufLen := 3

	return &RelayHandler{
		relay:  relay,
		recvCh: make(chan ClientMsgJSON, chanBufLen),
		sendCh: make(chan ServerMsg, chanBufLen),

		req:    req,
		realIP: realip.FromRequest(req),
		connID: connID,
	}
}

type RelayHandler struct {
	relay  *Relay
	recvCh chan ClientMsgJSON
	sendCh chan ServerMsg

	req    *http.Request
	realIP string
	connID string
}

func (rh *RelayHandler) HandleWebsocket(ctx context.Context, conn net.Conn) error {
	defer func() {
		if err := recover(); err != nil {
			logger.Panicw("paniced",
				"addr", rh.realIP,
				"conn_id", rh.connID,
				"error", err)
		}
	}()

	promActiveWebsocket.WithLabelValues(realip.FromRequest(rh.req), rh.connID).Inc()
	defer promActiveWebsocket.WithLabelValues(realip.FromRequest(rh.req), rh.connID).Dec()

	defer rh.relay.router.Delete(rh.connID)

	errCh := make(chan error, 2)

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- rh.wsSender(ctx, conn)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- rh.wsReceiver(ctx, conn)
	}()

	err := <-errCh

	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		if errors.Is(err, syscall.ECONNRESET) {
			return nil
		}

		return fmt.Errorf("handle websocket error: %w", err)
	}

	return nil
}

func (rh *RelayHandler) wsReceiver(
	ctx context.Context,
	conn net.Conn,
) error {
	lim := rate.NewLimiter(RateLimitRate, RateLimitBurst)
	reader := wsutil.NewServerSideReader(conn)

	for {
		if err := lim.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter returns error: %w", err)
		}

		payload, err := rh.wsRead(reader)
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		if !utf8.Valid(payload) {
			logger.Infow("payload is not utf8",
				"addr", rh.realIP,
				"conn_id", rh.connID,
				"payload", payload)
			continue
		}

		strMsg := string(payload)
		jsonMsg, err := ParseClientMsgJSON(strMsg)
		if err != nil {
			logger.Infow("received invalid msg",
				"addr", rh.realIP,
				"conn_id", rh.connID,
				"msg", strMsg,
				"error", err)
			continue
		}

		logger.Infow("recv msg",
			"addr", rh.realIP,
			"conn_id", rh.connID,
			"msg", strMsg)
		promWSRecvCounter.WithLabelValues(realip.FromRequest(rh.req), rh.connID, jsonMsg).Inc()

		switch msg := jsonMsg.(type) {
		case *ClientReqMsgJSON:
			if err := rh.serveClientReqMsgJSON(msg); err != nil {
				logger.Infow("failed to serve client req msg",
					"addr", rh.realIP,
					"conn_id", rh.connID,
					"error", err)
				continue
			}

		case *ClientCloseMsgJSON:
			if err := rh.serveClientCloseMsgJSON(msg); err != nil {
				logger.Infow("failed to serve client close msg",
					"addr", rh.realIP,
					"conn_id", rh.connID,
					"error", err)
				continue
			}

		case *ClientEventMsgJSON:
			if err := rh.serveClientEventMsgJSON(msg); err != nil {
				logger.Infow("failed to serve client event msg",
					"addr", rh.realIP,
					"conn_id", rh.connID,
					"error", err)
				continue
			}
		}
	}
}

func (*RelayHandler) wsRead(wsr *wsutil.Reader) ([]byte, error) {
	limit := *MaxClientMesLen + 1

	hdr, err := wsr.NextFrame()
	if err != nil {
		return nil, fmt.Errorf("failed to get next frame: %w", err)
	}
	if hdr.OpCode == ws.OpClose {
		return nil, io.EOF
	}

	r := io.LimitReader(wsr, int64(limit))
	res, err := io.ReadAll(r)
	if len(res) == limit {
		return res, fmt.Errorf("websocket message is too long (len=%v): %s", len(res), res)
	}
	return res, err
}

func (rh *RelayHandler) serveClientReqMsgJSON(
	msg *ClientReqMsgJSON,
) error {
	filters := NewFiltersFromFilterJSONs(msg.FilterJSONs)

	if len(filters) > MaxFilterLen+2 {
		return fmt.Errorf("filter is too long: %v", msg)
	}

	for _, event := range rh.relay.cache.FindAll(filters) {
		rh.sendCh <- NewServerEventMsg(msg.SubscriptionID, event)
	}
	rh.sendCh <- NewServerEOSEMsg(msg.SubscriptionID)

	// TODO(high-moctane) handle error, impl is not good
	if err := rh.relay.router.Subscribe(rh.connID, msg.SubscriptionID, filters, rh.sendCh); err != nil {
		return nil
	}
	return nil
}

func (rh *RelayHandler) serveClientCloseMsgJSON(msg *ClientCloseMsgJSON) error {
	if err := rh.relay.router.Close(rh.connID, msg.SubscriptionID); err != nil {
		return fmt.Errorf("cannot close conn %v", msg.SubscriptionID)
	}
	return nil
}

func (rh *RelayHandler) serveClientEventMsgJSON(msg *ClientEventMsgJSON) error {
	ok, err := msg.EventJSON.Verify()
	if err != nil {
		return fmt.Errorf("failed to verify event json: %v", msg)

	}
	if !ok {
		return fmt.Errorf("invalid signature: %v", msg)
	}

	promEventCounter.WithLabelValues(msg.EventJSON).Inc()

	event := NewEvent(msg.EventJSON, time.Now())

	if !event.ValidCreatedAt() {
		return fmt.Errorf("invalid created_at: %v", event.CreatedAtToTime())
	}

	rh.relay.cache.Save(event)

	if err := rh.relay.router.Publish(event); err != nil {
		return fmt.Errorf("failed to publish event: %v", event)
	}
	return nil
}

func (rh *RelayHandler) wsSender(
	ctx context.Context,
	conn net.Conn,
) (err error) {
	defer func() {
		if _, e := conn.Write(ws.CompiledCloseNormalClosure); e != nil {
			if errors.Is(e, net.ErrClosed) {
				return
			}
			err = fmt.Errorf("failed to send close frame: %w", e)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil

		case msg := <-rh.sendCh:
			promWSSendCounter.WithLabelValues(rh.realIP, rh.connID, msg).Inc()

			jsonMsg, err := msg.MarshalJSON()
			if err != nil {
				logger.Infow("failed to marshal server msg",
					"addr", rh.realIP,
					"conn_id", rh.connID,
					"msg", msg,
					"error", err)
				continue
			}

			if err := wsutil.WriteServerText(conn, jsonMsg); err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return fmt.Errorf("failed to write server text: %w", err)
			}

			logger.Infow("send msg",
				"addr", rh.realIP,
				"conn_id", rh.connID,
				"msg", string(jsonMsg))
		}
	}
}
