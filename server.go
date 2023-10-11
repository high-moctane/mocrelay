package mocrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var (
	ErrRelayStop = errors.New("relay stopped")
)

type Relay struct {
	Handler Handler

	opt *RelayOption

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

func (relay *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = ctxWithRealIP(ctx, r)
	ctx = ctxWithRequestID(ctx)
	r = r.WithContext(ctx)

	relay.logInfo(ctx, relay.logger, "mocrelay session start")

	errs := make(chan error, 3)
	defer func() {
		err := errors.Join(ErrRelayStop, <-errs, <-errs, <-errs)
		relay.logInfo(ctx, relay.logger, "mocrelay session end", "err", err)
	}()

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		relay.logInfo(ctx, relay.logger, "failed to upgrade http", "err", err)
		return
	}
	defer conn.Close()

	recv := make(chan ClientMsg)
	send := make(chan ServerMsg)

	go func() {
		defer cancel()
		defer close(recv)
		err := relay.serveRead(ctx, conn, recv, send)
		errs <- fmt.Errorf("serveRead terminated: %w", err)
	}()

	go func() {
		defer cancel()
		err := relay.serveWrite(ctx, conn, send)
		errs <- fmt.Errorf("serveWrite terminated: %w", err)
	}()

	go func() {
		defer cancel()
		err := relay.Handler.Handle(r, recv, send)
		errs <- fmt.Errorf("handler terminated: %w", err)
	}()

	<-ctx.Done()
}

func (relay *Relay) serveRead(
	ctx context.Context,
	conn net.Conn,
	recv chan<- ClientMsg,
	send chan ServerMsg,
) error {
	l := newRateLimiter(relay.recvRateLimitRate, relay.recvRateLimitBurst)
	defer l.Stop()

	r := wsutil.NewServerSideReader(conn)
	r.OnContinuation = wsutil.ControlFrameHandler(conn, ws.StateServerSide)
	r.OnIntermediate = wsutil.ControlFrameHandler(conn, ws.StateServerSide)

	for {
		payload, err := relay.readWebsocket(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.EOF
			}
			if errors.Is(err, ErrWebsocketMessageTooLong) {
				notice := NewServerNoticeMsg(
					fmt.Sprintf(
						"too long websocket message: limit is %d",
						relay.opt.maxMessageLength(),
					),
				)
				sendServerMsgCtx(ctx, send, notice)
				continue
			}
			return fmt.Errorf("failed to read websocket: %w", err)
		}
		if payload == nil {
			continue
		}

		msg, err := ParseClientMsg(payload)
		if err != nil {
			relay.logInfo(ctx, relay.recvLogger, "failed to parse client msg", "error", err)
			continue
		}

		relay.logInfo(
			ctx,
			relay.recvLogger,
			"recv client msg",
			"clientMsg",
			json.RawMessage(payload),
		)

		if msg, ok := msg.(*ClientEventMsg); ok {
			good, err := msg.Event.Verify()
			if err != nil {
				relay.logInfo(ctx, relay.recvLogger, "failed to verify event", "error", err)
				continue
			}
			if !good {
				relay.logInfo(ctx, relay.recvLogger, "invalid signature")
				continue
			}
		}

		select {
		case <-l.C:
			sendCtx(ctx, recv, msg)

		default:
			if m, ok := msg.(*ClientEventMsg); ok {
				sendCtx(
					ctx,
					send,
					ServerMsg(
						NewServerOKMsg(
							m.Event.ID,
							false,
							ServerOkMsgPrefixRateLimited,
							"slow down",
						),
					),
				)
				<-l.C
			} else {
				<-l.C
				sendCtx(ctx, recv, msg)
			}
		}
	}
}

var ErrWebsocketMessageTooLong = errors.New("too long websocket message")

func (relay *Relay) readWebsocket(r *wsutil.Reader) ([]byte, error) {
	hdr, err := r.NextFrame()
	if err != nil {
		return nil, fmt.Errorf("failed to read websocket header: %w", err)
	}

	if hdr.Length > relay.opt.maxMessageLength() {
		if err := r.Discard(); err != nil {
			return nil, fmt.Errorf("failed to discard unread websocket: %w", err)
		}
		return nil, ErrWebsocketMessageTooLong
	}
	if hdr.OpCode == ws.OpClose {
		if err := r.Discard(); err != nil {
			return nil, fmt.Errorf("failed to discard unread websocket: %w", err)
		}
		return nil, io.EOF
	}
	if hdr.OpCode != ws.OpText {
		if err := r.Discard(); err != nil {
			return nil, fmt.Errorf("failed to discard unread websocket: %w", err)
		}
		return nil, nil
	}

	payload := make([]byte, hdr.Length)
	n, err := r.Read(payload)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return payload, nil
		}
		return payload, err
	}
	if len(payload) != n {
		return payload, fmt.Errorf("invalid length of payload: %d", len(payload))
	}

	return payload, nil
}

func (relay *Relay) serveWrite(
	ctx context.Context,
	conn net.Conn,
	send <-chan ServerMsg,
) error {
	l := newRateLimiter(relay.sendRateLimitRate, 0)
	defer l.cancel()

	pingTicker := time.NewTicker(10 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("serverWrite terminated by ctx: %w", ctx.Err())

		case <-pingTicker.C:
			if _, err := conn.Write(ws.CompiledPing); err != nil {
				return fmt.Errorf("failed to send ping: %w", err)
			}

		case msg := <-send:
			<-l.C

			jsonMsg, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal server msg: %w", err)
			}

			if err := wsutil.WriteServerText(conn, jsonMsg); err != nil {
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

func (relay *Relay) prepareRateLimitOpts() {
	if relay.opt == nil {
		return
	}

	relay.recvRateLimitRate = relay.opt.RecvRateLimitRate
	relay.recvRateLimitBurst = relay.opt.RecvRateLimitBurst
	relay.sendRateLimitRate = relay.opt.SendRateLimitRate
}
