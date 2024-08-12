package mocrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"golang.org/x/time/rate"
)

var (
	ErrRelayStop = errors.New("relay stopped")
)

type Relay struct {
	Handler Handler

	opt RelayOption

	wg sync.WaitGroup
}

func NewRelay(handler Handler, option *RelayOption) *Relay {
	opt := option
	if opt == nil {
		opt = NewDefaultRelayOption()
	}

	relay := &Relay{
		Handler: handler,
		opt:     *opt,
	}

	return relay
}

func (relay *Relay) Wait() { relay.wg.Wait() }

func (relay *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relay.wg.Add(1)
	defer relay.wg.Done()

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = ctxWithRequest(ctx, r)

	relay.logInfo(ctx, relay.opt.Logger, "mocrelay session start")

	errs := make(chan error, 3)

	conn, err := websocket.Accept(
		w,
		r,
		&websocket.AcceptOptions{
			InsecureSkipVerify: true,
			CompressionMode:    websocket.CompressionDisabled,
		},
	)
	if err != nil {
		relay.logWarn(ctx, relay.opt.Logger, "failed to upgrade http", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")
	conn.SetReadLimit(relay.opt.MaxMessageLength)

	recv := make(chan ClientMsg)
	send := make(chan ServerMsg)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		defer close(recv)
		err := relay.serveReadLoop(ctx, conn, recv, send)
		errs <- fmt.Errorf("serveReadLoop terminated: %w", err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		err := relay.serveWriteLoop(ctx, conn, send)
		errs <- fmt.Errorf("serveWriteLoop terminated: %w", err)
	}()

	func() {
		defer cancel()
		err := relay.Handler.ServeNostr(ctx, send, recv)
		errs <- fmt.Errorf("handler terminated: %w", err)
	}()

	<-ctx.Done()

	conn.Close(websocket.StatusNormalClosure, "")

	wg.Wait()

	close(errs)
	for e := range errs {
		err = errors.Join(err, e)
	}
	err = errors.Join(ErrRelayStop, err)

	var wsErr websocket.CloseError

	if errors.Is(err, io.EOF) {
		relay.logInfo(ctx, relay.opt.Logger, "mocrelay session end")
	} else if errors.As(err, &wsErr) {
		relay.logInfo(ctx, relay.opt.Logger, "mocrelay session end with close", "code", wsErr.Code, "reason", wsErr.Reason)
	} else if errors.Is(err, context.Canceled) {
		relay.logInfo(ctx, relay.opt.Logger, "mocrelay session end with cancel")
	} else {
		relay.logWarn(ctx, relay.opt.Logger, "mocrelay session end with error", "err", err)
	}
}

func (relay *Relay) serveReadLoop(
	ctx context.Context,
	conn *websocket.Conn,
	recv chan<- ClientMsg,
	send chan ServerMsg,
) error {
	l := rate.NewLimiter(rate.Limit(relay.opt.RecvRateLimitRate), relay.opt.RecvRateLimitBurst)

	for {
		if err := relay.serveRead(ctx, conn, recv, send, l); err != nil {
			return fmt.Errorf("serveRead terminated: %w", err)
		}
	}
}

func (relay *Relay) serveRead(
	ctx context.Context,
	conn *websocket.Conn,
	recv chan<- ClientMsg,
	send chan ServerMsg,
	limiter *rate.Limiter,
) error {
	if err := limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}

	typ, payload, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read websocket: %w", err)
	}
	if typ != websocket.MessageText {
		relay.logWarn(ctx, relay.opt.Logger, "received binary websocket message")
		notice := NewServerNoticeMsgf("binary websocket message type is not allowed")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}
	if !utf8.Valid(payload) || !json.Valid(payload) {
		relay.logWarn(ctx, relay.opt.Logger, "received invalid json message")
		notice := NewServerNoticeMsgf("invalid json msg")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}

	msg, err := ParseClientMsg(payload)
	if err != nil {
		relay.logWarn(ctx, relay.opt.Logger, "failed to parse client msg", "error", err)
		notice := NewServerNoticeMsgf("invalid client msg")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}

	if !ValidClientMsg(msg) {
		relay.logWarn(ctx, relay.opt.Logger, "invalid client msg", "error", err)
		notice := NewServerNoticeMsgf("invalid client msg: %s", payload)
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}
	if msg, ok := msg.(*ClientEventMsg); ok {
		valid, err := msg.Event.Verify()
		if err != nil {
			relay.logWarn(ctx, relay.opt.Logger, "failed to verify event msg", "error", err)
			notice := NewServerNoticeMsg("internal error")
			sendServerMsgCtx(ctx, send, notice)
			return nil
		}
		if !valid {
			relay.logWarn(ctx, relay.opt.Logger, "received invalid sig event", "clientMsg", msg)
			notice := NewServerNoticeMsgf("invalid sig event: %s", msg.Event.ID)
			sendServerMsgCtx(ctx, send, notice)
			return nil
		}
	}

	sendCtx(ctx, recv, msg)

	return nil
}

func (relay *Relay) serveWriteLoop(
	ctx context.Context,
	conn *websocket.Conn,
	send <-chan ServerMsg,
) error {
	var pingTickCh <-chan time.Time
	if relay.opt.PingDuration != 0 {
		pingTicker := time.NewTicker(relay.opt.PingDuration)
		defer pingTicker.Stop()

		pingTickCh = pingTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("serverWrite terminated by ctx: %w", ctx.Err())

		case <-pingTickCh:
			if err := relay.sendPingWithTimeout(ctx, conn); err != nil {
				return fmt.Errorf("failed to send ping: %w", err)
			}

		case msg := <-send:
			jsonMsg, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal server msg: %w", err)
			}

			if err := relay.sendMsgWithTimeout(ctx, conn, jsonMsg); err != nil {
				return fmt.Errorf("failed to write websocket: %w", err)
			}
		}
	}
}

func (relay *Relay) sendPingWithTimeout(ctx context.Context, conn *websocket.Conn) error {
	if relay.opt.PingDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, relay.opt.SendTimeout)
		defer cancel()
	}
	return conn.Ping(ctx)
}

func (relay *Relay) sendMsgWithTimeout(
	ctx context.Context,
	conn *websocket.Conn,
	msg []byte,
) error {
	if relay.opt.PingDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, relay.opt.SendTimeout)
		defer cancel()
	}
	return conn.Write(ctx, websocket.MessageText, msg)
}

func (relay *Relay) logInfo(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.InfoContext(ctx, msg, args...)
}

func (relay *Relay) logWarn(ctx context.Context, logger *slog.Logger, msg string, args ...any) {
	if logger == nil {
		return
	}
	logger.WarnContext(ctx, msg, args...)
}

type RelayOption struct {
	Logger *slog.Logger

	SendTimeout time.Duration

	RecvRateLimitRate  float64
	RecvRateLimitBurst int

	MaxMessageLength int64

	PingDuration time.Duration
}

func NewDefaultRelayOption() *RelayOption {
	return &RelayOption{
		SendTimeout: 10 * time.Second,

		RecvRateLimitRate:  10,
		RecvRateLimitBurst: 10,

		MaxMessageLength: 100_000,

		PingDuration: 1 * time.Minute,
	}
}
