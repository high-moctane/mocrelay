package mocrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
)

var (
	ErrRelayStop = errors.New("relay stopped")
)

type Relay struct {
	Handler Handler
}

func (relay *Relay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(high-moctane) Use slog

	ctx := r.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx = ctxWithSessionID(ctx)
	r = r.WithContext(ctx)

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	errs := make(chan error, 2)
	recv := make(chan ClientMsg, 1)
	send := make(chan ServerMsg, 1)

	go func() {
		defer close(recv)
		err := relay.serveRead(ctx, conn, recv)
		errs <- fmt.Errorf("serveRead terminated: %w", err)
	}()

	go func() {
		err := relay.serveWrite(ctx, conn, send)
		errs <- fmt.Errorf("serveWrite terminated: %w", err)
	}()

	err = relay.Handler.Handle(r, recv, send)

	cancel()
	err = fmt.Errorf("handler terminated: %w", err)
	err = errors.Join(err, <-errs, <-errs, ErrRelayStop)

	log.Println(err)
}

func (relay *Relay) serveRead(
	ctx context.Context,
	conn net.Conn,
	recv chan<- ClientMsg,
) error {
	// TODO(high-moctane) rate-limit

	for {
		payload, err := wsutil.ReadClientText(conn)
		if err != nil {
			return fmt.Errorf("failed to read websocket: %w", err)
		}

		msg, err := ParseClientMsg(payload)
		if err != nil {
			return fmt.Errorf("failed to parse client msg: %w", err)
		}

		recv <- msg
	}
}

func (relay *Relay) serveWrite(
	ctx context.Context,
	conn net.Conn,
	send <-chan ServerMsg,
) error {
	// TODO(high-moctane) circuit braker

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
			jsonMsg, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("failed to marshal server msg: %w", err)
			}

			if err := wsutil.WriteServerText(conn, jsonMsg); err != nil {
				return fmt.Errorf("failed to write websocket: %w", err)
			}
		}
	}
}

type sessionIDKey struct{}

func ctxWithSessionID(ctx context.Context) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, uuid.NewString())
}

func GetSessionID(ctx context.Context) string {
	return ctx.Value(sessionIDKey{}).(string)
}
