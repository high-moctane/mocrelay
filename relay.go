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

	"nhooyr.io/websocket"
)

var (
	ErrRelayStop = errors.New("relay stopped")
)

type Relay struct {
	Handler Handler

	opt *RelayOption

	wg sync.WaitGroup

	logger     *slog.Logger
	recvLogger *slog.Logger
	sendLogger *slog.Logger

	recvRateLimitRate  time.Duration
	recvRateLimitBurst int
	sendRateLimitRate  time.Duration
}

type RelayOption struct {
	Logger     *slog.Logger
	RecvLogger *slog.Logger
	SendLogger *slog.Logger

	RecvRateLimitRate  time.Duration
	RecvRateLimitBurst int
	SendRateLimitRate  time.Duration

	MaxMessageLength int64

	PingDuration time.Duration
}

func (opt *RelayOption) maxMessageLength() int64 {
	const defaultMaxMessageLength = 16384

	if opt == nil || opt.MaxMessageLength == 0 {
		return defaultMaxMessageLength
	}

	return opt.MaxMessageLength
}

func NewRelay(handler Handler, option *RelayOption) *Relay {
	relay := &Relay{
		Handler: handler,
		opt:     option,
	}

	relay.prepareLoggers()
	relay.prepareRateLimitOpts()

	return relay
}

func (relay *Relay) Wait() { relay.wg.Wait() }

func (relay *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	relay.wg.Add(1)
	defer relay.wg.Done()

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = ctxWithRealIP(ctx, r)
	ctx = ctxWithRequestID(ctx)
	ctx = ctxWithHTTPHeader(ctx, r)
	r = r.WithContext(ctx)

	relay.logInfo(ctx, relay.logger, "mocrelay session start")

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
		relay.logWarn(ctx, relay.logger, "failed to upgrade http", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")
	conn.SetReadLimit(relay.opt.maxMessageLength())

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

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		err := relay.Handler.Handle(r, recv, send)
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

	if errors.Is(err, io.EOF) {
		relay.logInfo(ctx, relay.logger, "mocrelay session end")
	} else {
		relay.logWarn(ctx, relay.logger, "mocrelay session end with error", "err", err)
	}
}

func (relay *Relay) serveReadLoop(
	ctx context.Context,
	conn *websocket.Conn,
	recv chan<- ClientMsg,
	send chan ServerMsg,
) error {
	l := newRateLimiter(relay.recvRateLimitRate, relay.recvRateLimitBurst)
	defer l.Stop()

	for {
		if err := relay.serveRead(ctx, conn, recv, send, l); err != nil {
			return err
		}
	}
}

func (relay *Relay) serveRead(
	ctx context.Context,
	conn *websocket.Conn,
	recv chan<- ClientMsg,
	send chan ServerMsg,
	limiter *rateLimiter,
) error {
	typ, payload, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read websocket: %w", err)
	}
	if typ != websocket.MessageText {
		relay.logWarn(ctx, relay.recvLogger, "received binary websocket message")
		notice := NewServerNoticeMsgf("binary websocket message type is not allowed")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}
	if !utf8.Valid(payload) || !json.Valid(payload) {
		relay.logWarn(ctx, relay.recvLogger, "received invalid json message")
		notice := NewServerNoticeMsgf("invalid json msg")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}

	msg, err := ParseClientMsg(payload)
	if err != nil {
		relay.logWarn(ctx, relay.recvLogger, "failed to parse client msg", "error", err)
		notice := NewServerNoticeMsgf("invalid client msg")
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}

	relay.logInfo(
		ctx,
		relay.recvLogger,
		"recv client msg",
		"clientMsg",
		json.RawMessage(payload),
	)

	if ok := ValidClientMsg(msg); !ok {
		relay.logWarn(ctx, relay.recvLogger, "invalid client msg", "error", err)
		notice := NewServerNoticeMsgf("invalid client msg: %s", payload)
		sendServerMsgCtx(ctx, send, notice)
		return nil
	}
	if msg, ok := msg.(*ClientEventMsg); ok {
		valid, err := msg.Event.Verify()
		if err != nil {
			relay.logWarn(ctx, relay.recvLogger, "failed to verify event msg", "error", err)
			notice := NewServerNoticeMsg("internal error")
			sendServerMsgCtx(ctx, send, notice)
			return nil
		}
		if !valid {
			relay.logWarn(ctx, relay.recvLogger, "received invalid sig event", "clientMsg", msg)
			notice := NewServerNoticeMsgf("invalid sig event: %s", msg.Event.ID)
			sendServerMsgCtx(ctx, send, notice)
			return nil
		}
	}

	<-limiter.C

	sendCtx(ctx, recv, msg)

	return nil
}

func (relay *Relay) serveWriteLoop(
	ctx context.Context,
	conn *websocket.Conn,
	send <-chan ServerMsg,
) error {
	l := newRateLimiter(relay.sendRateLimitRate, 0)
	defer l.cancel()

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
			if err := conn.Ping(ctx); err != nil {
				return fmt.Errorf("failed to send ping: %w", err)
			}

		case msg := <-send:
			<-l.C

			jsonMsg, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal server msg: %w", err)
			}

			if err := conn.Write(ctx, websocket.MessageText, jsonMsg); err != nil {
				return fmt.Errorf("failed to write websocket: %w", err)
			}

			relay.logInfo(
				ctx,
				relay.sendLogger,
				"sent server msg",
				"serverMsg",
				json.RawMessage(jsonMsg),
			)
		}
	}
}

func (relay *Relay) prepareLoggers() {
	if relay.opt == nil {
		return
	}

	if relay.opt.Logger != nil {
		relay.logger = slog.New(WithSlogMocrelayHandler(relay.opt.Logger.Handler()))
	}
	if relay.opt.RecvLogger != nil {
		relay.recvLogger = slog.New(WithSlogMocrelayHandler(relay.opt.RecvLogger.Handler()))
	}
	if relay.opt.SendLogger != nil {
		relay.sendLogger = slog.New(WithSlogMocrelayHandler(relay.opt.SendLogger.Handler()))
	}
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

func (relay *Relay) prepareRateLimitOpts() {
	if relay.opt == nil {
		return
	}

	relay.recvRateLimitRate = relay.opt.RecvRateLimitRate
	relay.recvRateLimitBurst = relay.opt.RecvRateLimitBurst
	relay.sendRateLimitRate = relay.opt.SendRateLimitRate
}
