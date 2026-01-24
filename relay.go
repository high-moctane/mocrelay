//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"unicode/utf8"

	"github.com/coder/websocket"
)

// Relay wraps a Handler to serve it over HTTP/WebSocket.
// It implements http.Handler.
type Relay struct {
	Handler Handler
	Logger  *slog.Logger

	// MaxMessageLength is the maximum size of a WebSocket message.
	// Default: 100KB
	MaxMessageLength int64

	// Info is the NIP-11 Relay Information Document.
	// If set, the relay will respond to HTTP requests with
	// Accept: application/nostr+json header.
	Info *RelayInfo

	wg sync.WaitGroup
}

// NewRelay creates a new Relay with the given handler.
func NewRelay(handler Handler) *Relay {
	return &Relay{
		Handler:          handler,
		MaxMessageLength: 100_000,
	}
}

// Wait blocks until all connections have finished.
func (r *Relay) Wait() {
	r.wg.Wait()
}

// ServeHTTP implements http.Handler.
func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// NIP-11: Respond with relay info if Accept header is application/nostr+json
	if r.Info != nil && req.Header.Get("Accept") == "application/nostr+json" {
		r.serveNIP11(w, req)
		return
	}

	r.wg.Add(1)
	defer r.wg.Done()

	ctx := req.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	r.logInfo(ctx, "connection start")

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		r.logWarn(ctx, "failed to upgrade to websocket", "error", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")

	if r.MaxMessageLength > 0 {
		conn.SetReadLimit(r.MaxMessageLength)
	}

	recv := make(chan *ClientMsg)
	send := make(chan *ServerMsg)

	errs := make(chan error, 3)
	var wg sync.WaitGroup

	// Read loop: WebSocket -> recv channel
	wg.Go(func() {
		defer cancel()
		defer close(recv)
		err := r.readLoop(ctx, conn, recv, send)
		errs <- fmt.Errorf("readLoop: %w", err)
	})

	// Write loop: send channel -> WebSocket
	wg.Go(func() {
		defer cancel()
		err := r.writeLoop(ctx, conn, send)
		errs <- fmt.Errorf("writeLoop: %w", err)
	})

	// Handler
	wg.Go(func() {
		defer cancel()
		err := r.Handler.ServeNostr(ctx, send, recv)
		errs <- fmt.Errorf("handler: %w", err)
	})

	// Wait for cancellation
	<-ctx.Done()
	conn.Close(websocket.StatusNormalClosure, "")

	wg.Wait()
	close(errs)

	// Collect errors for logging
	var allErrs error
	for e := range errs {
		allErrs = errors.Join(allErrs, e)
	}

	var wsErr websocket.CloseError
	if errors.Is(allErrs, io.EOF) {
		r.logInfo(ctx, "connection end")
	} else if errors.As(allErrs, &wsErr) {
		r.logInfo(ctx, "connection end", "code", wsErr.Code, "reason", wsErr.Reason)
	} else if errors.Is(allErrs, context.Canceled) {
		r.logInfo(ctx, "connection end (canceled)")
	} else {
		r.logWarn(ctx, "connection end with error", "error", allErrs)
	}
}

// readLoop reads messages from WebSocket and sends them to recv channel.
func (r *Relay) readLoop(
	ctx context.Context,
	conn *websocket.Conn,
	recv chan<- *ClientMsg,
	send chan<- *ServerMsg,
) error {
	for {
		typ, payload, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		// Must be text message
		if typ != websocket.MessageText {
			r.logWarn(ctx, "received binary message")
			r.sendNotice(ctx, send, "binary message not allowed")
			continue
		}

		// Must be valid UTF-8
		if !utf8.Valid(payload) {
			r.logWarn(ctx, "received invalid UTF-8")
			r.sendNotice(ctx, send, "invalid UTF-8")
			continue
		}

		// Parse client message
		msg, err := ParseClientMsg(payload)
		if err != nil {
			r.logWarn(ctx, "failed to parse client message", "error", err)
			r.sendNotice(ctx, send, "invalid message format")
			continue
		}

		// Verify event signature if EVENT message
		if msg.Type == MsgTypeEvent && msg.Event != nil {
			valid, err := msg.Event.Verify()
			if err != nil {
				r.logWarn(ctx, "failed to verify event", "error", err)
				r.sendNotice(ctx, send, "verification error")
				continue
			}
			if !valid {
				r.logWarn(ctx, "invalid event signature", "id", msg.Event.ID)
				r.sendNotice(ctx, send, "invalid signature")
				continue
			}
		}

		// Send to handler
		select {
		case <-ctx.Done():
			return ctx.Err()
		case recv <- msg:
		}
	}
}

// writeLoop reads messages from send channel and writes them to WebSocket.
func (r *Relay) writeLoop(
	ctx context.Context,
	conn *websocket.Conn,
	send <-chan *ServerMsg,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case msg, ok := <-send:
			if !ok {
				return nil
			}

			data, err := json.Marshal(msg)
			if err != nil {
				r.logWarn(ctx, "failed to marshal server message", "error", err)
				continue
			}

			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return err
			}
		}
	}
}

// sendNotice sends a NOTICE message to the client.
func (r *Relay) sendNotice(ctx context.Context, send chan<- *ServerMsg, message string) {
	select {
	case <-ctx.Done():
	case send <- NewServerNoticeMsg(message):
	}
}

func (r *Relay) logInfo(ctx context.Context, msg string, args ...any) {
	if r.Logger != nil {
		r.Logger.InfoContext(ctx, msg, args...)
	}
}

func (r *Relay) logWarn(ctx context.Context, msg string, args ...any) {
	if r.Logger != nil {
		r.Logger.WarnContext(ctx, msg, args...)
	}
}

// serveNIP11 responds with the NIP-11 Relay Information Document.
func (r *Relay) serveNIP11(w http.ResponseWriter, req *http.Request) {
	// CORS headers (required by NIP-11)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Accept")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")

	// Handle preflight request
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Only GET is allowed for NIP-11
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/nostr+json")

	data, err := json.Marshal(r.Info)
	if err != nil {
		r.logWarn(req.Context(), "failed to marshal relay info", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Write(data)
}
