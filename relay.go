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
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

const (
	RateLimitRate  = 20
	RateLimitBurst = 10
)

func RelayAccessHandlerFunc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("failed to upgrade HTTP")
		return
	}
	defer conn.Close()

	log.Ctx(ctx).Info().Msg("connect websocket")
	defer log.Ctx(ctx).Info().Msg("disconnect websocket")

	wreq := NewWebsocketRequest(r, conn)

	if err := DefaultRelay.NewHandler().Serve(ctx, wreq); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("websocket error")
	}
}

func NewWebsocketRequest(req *http.Request, conn net.Conn) *WebsocketRequest {
	return &WebsocketRequest{
		HTTPReq: req,
		Conn:    conn,
	}
}

type WebsocketRequest struct {
	HTTPReq *http.Request
	Conn    net.Conn
}

var DefaultRelay = NewRelay()

func NewRelay() *Relay {
	return &Relay{
		router: NewRouter(DefaultFilters, Cfg.MaxSubscriptions),
		cache:  NewCache(Cfg.CacheSize, DefaultFilters),
	}
}

type Relay struct {
	router *Router
	cache  *Cache
}

func (relay *Relay) NewHandler() *RelayHandler {
	chanBufLen := 3

	return &RelayHandler{
		relay:  relay,
		recvCh: NewTryChan[ClientMsgJSON](chanBufLen),
		sendCh: NewTryChan[ServerMsg](chanBufLen),
	}
}

type RelayHandler struct {
	relay  *Relay
	recvCh TryChan[ClientMsgJSON]
	sendCh TryChan[ServerMsg]
}

func (rh *RelayHandler) Serve(ctx context.Context, r *WebsocketRequest) error {
	defer func() {
		if err := recover(); err != nil {
			log.Ctx(ctx).Panic().Any("error", err).Msg("paniced")
		}
	}()

	promActiveWebsocket.WithLabelValues(ctx).Inc()
	defer promActiveWebsocket.WithLabelValues(ctx).Dec()

	defer rh.relay.router.Delete(GetCtxConnID(ctx))

	errCh := make(chan error, 2)

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- rh.wsSender(ctx, r)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errCh <- rh.wsReceiver(ctx, r)
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
	r *WebsocketRequest,
) error {
	lim := rate.NewLimiter(RateLimitRate, RateLimitBurst)
	reader := wsutil.NewServerSideReader(r.Conn)

	for {
		if err := lim.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter returns error: %w", err)
		}

		payload, err := rh.wsRead(reader)
		if err != nil {
			return fmt.Errorf("receive error: %w", err)
		}

		if !utf8.Valid(payload) {
			log.Ctx(ctx).Info().Bytes("payload", payload).Msg("payload is not utf8")
			continue
		}

		jsonMsg, err := ParseClientMsgJSON(payload)
		if err != nil {
			log.Ctx(ctx).Info().Err(err).Str("payload", string(payload)).Msg("received invalid client msg")
			continue
		}

		ctx := log.Ctx(ctx).With().RawJSON("client_msg", payload).Logger().WithContext(ctx)
		log.Ctx(ctx).Info().Msg("receive client msg")
		promWSRecvCounter.WithLabelValues(ctx, jsonMsg).Inc()

		switch msg := jsonMsg.(type) {
		case *ClientReqMsgJSON:
			if err := rh.serveClientReqMsgJSON(ctx, r, msg); err != nil {
				log.Ctx(ctx).Info().Err(err).Msg("failed to serve client req msg")
				continue
			}

		case *ClientCloseMsgJSON:
			if err := rh.serveClientCloseMsgJSON(ctx, r, msg); err != nil {
				log.Ctx(ctx).Info().Err(err).Msg("failed to serve client close msg")
				continue
			}

		case *ClientEventMsgJSON:
			if err := rh.serveClientEventMsgJSON(ctx, r, msg); err != nil {
				log.Ctx(ctx).Info().Err(err).Msg("failed to serve client event msg")
				continue
			}
		}
	}
}

func (*RelayHandler) wsRead(wsr *wsutil.Reader) ([]byte, error) {
	limit := Cfg.MaxMessageLength + 1

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
	ctx context.Context,
	r *WebsocketRequest,
	msg *ClientReqMsgJSON,
) error {
	filters := NewFiltersFromFilterJSONs(msg.FilterJSONs)

	if len(filters) > Cfg.MaxFilters+2 {
		return fmt.Errorf("filter is too long: %v", msg)
	}

	for _, event := range rh.relay.cache.FindAll(filters) {
		rh.TryEnqueueServerMsg(NewServerEventMsg(msg.SubscriptionID, event))
	}
	rh.TryEnqueueServerMsg(NewServerEOSEMsg(msg.SubscriptionID))

	// TODO(high-moctane) handle error, impl is not good
	sendFunc := func(msg ServerMsg) bool { return rh.TryEnqueueServerMsg(msg) }
	if err := rh.relay.router.Subscribe(GetCtxConnID(ctx), msg.SubscriptionID, filters, sendFunc); err != nil {
		return nil
	}
	return nil
}

func (rh *RelayHandler) serveClientCloseMsgJSON(
	ctx context.Context,
	r *WebsocketRequest,
	msg *ClientCloseMsgJSON,
) error {
	if err := rh.relay.router.Close(GetCtxConnID(ctx), msg.SubscriptionID); err != nil {
		return fmt.Errorf("cannot close conn %v", msg.SubscriptionID)
	}
	return nil
}

func (rh *RelayHandler) serveClientEventMsgJSON(
	ctx context.Context,
	r *WebsocketRequest,
	msg *ClientEventMsgJSON,
) error {
	ok, err := msg.EventJSON.Verify()
	if err != nil {
		return fmt.Errorf("failed to verify event json: %v", msg)

	}
	if !ok {
		rh.TryEnqueueServerMsg(NewServerOKMsg(msg.EventJSON.ID, false, ServerOKMsgPrefixInvalid, "signature is wrong"))
		return nil
	}

	promEventCounter.WithLabelValues(msg.EventJSON).Inc()

	event := NewEvent(msg.EventJSON, time.Now())

	if !event.ValidCreatedAt() {
		rh.TryEnqueueServerMsg(NewServerOKMsg(event.ID, false, ServerOKMsgPrefixInvalid, "created_at is too far off"))
		return nil
	}

	if ok := rh.relay.cache.Save(event); ok {
		rh.TryEnqueueServerMsg(NewServerOKMsg(event.ID, true, "", ""))
	} else {
		rh.TryEnqueueServerMsg(NewServerOKMsg(event.ID, false, ServerOKMsgPrefixDuplicate, "the event has already been saved"))
	}

	if err := rh.relay.router.Publish(event); err != nil {
		return fmt.Errorf("failed to publish event: %v", event)
	}
	return nil
}

func (rh *RelayHandler) wsSender(
	ctx context.Context,
	r *WebsocketRequest,
) (err error) {
	defer func() {
		if _, e := r.Conn.Write(ws.CompiledCloseNormalClosure); e != nil {
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
			promWSSendCounter.WithLabelValues(ctx, msg).Inc()

			jsonMsg, err := msg.MarshalJSON()
			if err != nil {
				log.Ctx(ctx).Info().Err(err).Msg("failed to marshal server msg")
				continue
			}

			if err := wsutil.WriteServerText(r.Conn, jsonMsg); err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return fmt.Errorf("failed to write server text: %w", err)
			}

			log.Ctx(ctx).Debug().RawJSON("server_msg", jsonMsg).Msg("send server msg")
		}
	}
}

func (rh *RelayHandler) TryEnqueueServerMsg(msg ServerMsg) bool {
	ok := rh.sendCh.TrySend(msg)
	if !ok {
		promTryEnqueueServerMsgFail.Inc()
	}
	return ok
}
