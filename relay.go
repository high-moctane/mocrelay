package mocrelay

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
)

// Relay wraps a Handler to serve it over HTTP/WebSocket.
// It implements http.Handler.
type Relay struct {
	handler          Handler
	logger           *slog.Logger
	maxMessageLength int64
	pingInterval     time.Duration
	pingTimeout      time.Duration
	info             *RelayInfo
	metrics          *RelayMetrics
	rejectionMetrics *RejectionMetrics
	connIDFunc       func() string

	mu      sync.Mutex
	wg      sync.WaitGroup
	connID  uint64
	cancels map[uint64]context.CancelFunc
	closed  bool
}

// RelayOptions configures Relay behavior.
// All fields are optional; the zero value gives sensible defaults.
type RelayOptions struct {
	// Logger is the structured logger for connection and error events.
	// Default: slog.Default()
	Logger *slog.Logger

	// MaxMessageLength is the maximum size of a WebSocket message in bytes.
	// Set to a negative value to disable the limit.
	// Default: 100_000 (100 KB)
	MaxMessageLength int64

	// PingInterval is the interval between WebSocket pings.
	// Set to a negative value to disable pings.
	// Default: 30 seconds
	PingInterval time.Duration

	// PingTimeout is the timeout for WebSocket ping responses and write operations.
	// If a pong or write does not complete within this duration, the connection is closed.
	// Default: 10 seconds
	PingTimeout time.Duration

	// Info is the NIP-11 Relay Information Document.
	// If set, the relay will respond to HTTP requests with
	// Accept: application/nostr+json header.
	Info *RelayInfo

	// Metrics is the Prometheus metrics collector.
	// If set, the relay will collect connection and message metrics.
	Metrics *RelayMetrics

	// RejectionMetrics is the unified rejection counter shared across every
	// middleware. If set, the relay installs it into the request context so
	// that logRejection (called from every middleware drop) auto-increments
	// the counter alongside its structured log entry. If nil, rejections are
	// only logged — no counter is incremented.
	RejectionMetrics *RejectionMetrics

	// ConnIDFunc generates a unique connection ID string.
	// If nil, a default monotonic counter ("1", "2", ...) is used.
	ConnIDFunc func() string
}

// NewRelay creates a new Relay with the given handler.
// opts can be nil for default options.
func NewRelay(handler Handler, opts *RelayOptions) *Relay {
	if opts == nil {
		opts = &RelayOptions{}
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	maxMessageLength := opts.MaxMessageLength
	if maxMessageLength == 0 {
		maxMessageLength = 100_000
	}

	pingInterval := opts.PingInterval
	if pingInterval == 0 {
		pingInterval = 30 * time.Second
	}

	pingTimeout := opts.PingTimeout
	if pingTimeout == 0 {
		pingTimeout = 10 * time.Second
	}

	return &Relay{
		handler:          handler,
		logger:           logger,
		maxMessageLength: maxMessageLength,
		pingInterval:     pingInterval,
		pingTimeout:      pingTimeout,
		info:             opts.Info,
		metrics:          opts.Metrics,
		rejectionMetrics: opts.RejectionMetrics,
		connIDFunc:       opts.ConnIDFunc,
	}
}

// Wait blocks until all connections have finished.
func (r *Relay) Wait() {
	r.wg.Wait()
}

// Shutdown gracefully shuts down the relay.
// It closes all WebSocket connections and waits for them to finish.
// If ctx is canceled before all connections finish, it returns ctx.Err().
func (r *Relay) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	numConns := len(r.cancels)
	for _, cancel := range r.cancels {
		cancel()
	}
	r.mu.Unlock()

	r.logger.InfoContext(ctx, "relay: shutdown initiated", "active_conns", numConns)

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.InfoContext(ctx, "relay: shutdown complete")
		return nil
	case <-ctx.Done():
		r.logger.WarnContext(ctx, "relay: shutdown deadline exceeded, some connections may not have drained")
		return ctx.Err()
	}
}

func (r *Relay) registerConn(cancel context.CancelFunc) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cancels == nil {
		r.cancels = make(map[uint64]context.CancelFunc)
	}

	r.connID++
	id := r.connID
	r.cancels[id] = cancel
	return id
}

func (r *Relay) unregisterConn(id uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, id)
}

// ServeHTTP implements http.Handler.
func (r *Relay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Check if relay is shutting down (for all requests)
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()

	if closed {
		http.Error(w, "relay is shutting down", http.StatusServiceUnavailable)
		return
	}

	// NIP-11: Respond with relay info if Accept header is application/nostr+json
	if r.info != nil && req.Header.Get("Accept") == "application/nostr+json" {
		r.serveNIP11(w, req)
		return
	}

	// Non-WebSocket GET: return simple message instead of upgrade error
	if req.Header.Get("Upgrade") == "" {
		r.serveWelcome(w, req)
		return
	}

	// WebSocket connection: track with WaitGroup
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		http.Error(w, "relay is shutting down", http.StatusServiceUnavailable)
		return
	}
	r.wg.Add(1)
	r.mu.Unlock()

	defer r.wg.Done()

	ctx := req.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	internalID := r.registerConn(cancel)
	defer r.unregisterConn(internalID)

	var connID string
	if r.connIDFunc != nil {
		connID = r.connIDFunc()
	} else {
		connID = strconv.FormatUint(internalID, 10)
	}

	ctx = contextWithConnID(ctx, connID)
	logger := r.logger.With("conn_id", connID)
	ctx = ContextWithLogger(ctx, logger)
	if r.rejectionMetrics != nil {
		ctx = ContextWithRejectionMetrics(ctx, r.rejectionMetrics)
	}

	// Metrics: connection tracking
	if r.metrics != nil {
		r.metrics.ConnectionsTotal.Inc()
		r.metrics.ConnectionsCurrent.Inc()
		defer r.metrics.ConnectionsCurrent.Dec()
	}

	logger.InfoContext(ctx, "connection start")

	// Upgrade to WebSocket
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to upgrade to websocket", "error", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "")

	if r.maxMessageLength > 0 {
		conn.SetReadLimit(r.maxMessageLength)
	}

	recv := make(chan *ClientMsg)
	send := make(chan *ServerMsg, 128)

	errs := make(chan error, 2)
	var wg sync.WaitGroup

	// Read loop: WebSocket -> recv channel
	wg.Go(func() {
		defer cancel()
		defer close(recv)
		err := r.readLoop(ctx, conn, recv, send)
		errs <- fmt.Errorf("readLoop: %w", err)
	})

	// Write loop: send channel -> WebSocket + ping
	wg.Go(func() {
		defer cancel()
		err := r.writeLoop(ctx, conn, send)
		errs <- fmt.Errorf("writeLoop: %w", err)
	})

	// Run handler in current goroutine (saves 1 goroutine)
	handlerErr := r.handler.ServeNostr(ctx, send, recv)
	cancel()
	conn.Close(websocket.StatusNormalClosure, "")

	wg.Wait()
	close(errs)

	// Collect errors for logging
	allErrs := fmt.Errorf("handler: %w", handlerErr)
	for e := range errs {
		allErrs = errors.Join(allErrs, e)
	}

	var wsErr websocket.CloseError
	if errors.Is(allErrs, io.EOF) {
		logger.InfoContext(ctx, "connection end")
	} else if errors.As(allErrs, &wsErr) {
		logger.InfoContext(ctx, "connection end", "code", wsErr.Code, "reason", wsErr.Reason)
	} else if errors.Is(allErrs, context.Canceled) {
		logger.InfoContext(ctx, "connection end (canceled)")
	} else {
		logger.WarnContext(ctx, "connection end with error", "error", allErrs)
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
			LoggerFromContext(ctx).WarnContext(ctx, "received binary message")
			r.sendNotice(ctx, send, "binary message not allowed")
			continue
		}

		// Must be valid UTF-8
		if !utf8.Valid(payload) {
			LoggerFromContext(ctx).WarnContext(ctx, "received invalid UTF-8")
			r.sendNotice(ctx, send, "invalid UTF-8")
			continue
		}

		// Parse client message
		msg, err := ParseClientMsg(payload)
		if err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "failed to parse client message", "error", err)
			r.sendNotice(ctx, send, "invalid message format")
			continue
		}

		// Verify event signature if EVENT message
		if msg.Type == MsgTypeEvent && msg.Event != nil {
			valid, err := msg.Event.Verify()
			if err != nil {
				LoggerFromContext(ctx).WarnContext(ctx, "failed to verify event", "error", err)
				r.sendNotice(ctx, send, "verification error")
				continue
			}
			if !valid {
				LoggerFromContext(ctx).WarnContext(ctx, "invalid event signature", "id", msg.Event.ID)
				r.sendNotice(ctx, send, "invalid signature")
				continue
			}
		}

		// Metrics: message received
		if r.metrics != nil {
			r.metrics.MessagesReceived.WithLabelValues(string(msg.Type)).Inc()
			if msg.Type == MsgTypeEvent && msg.Event != nil {
				kind, typ := kindLabels(msg.Event.Kind)
				r.metrics.EventsReceived.WithLabelValues(kind, typ).Inc()
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

// writeLoop reads messages from send channel, writes them to WebSocket,
// and sends periodic pings to detect dead connections.
func (r *Relay) writeLoop(
	ctx context.Context,
	conn *websocket.Conn,
	send <-chan *ServerMsg,
) error {
	// Ping setup (nil channel pattern: tickerC stays nil when pings disabled)
	var tickerC <-chan time.Time
	if r.pingInterval > 0 {
		ticker := time.NewTicker(r.pingInterval)
		defer ticker.Stop()
		tickerC = ticker.C
	}

	timeout := r.pingTimeout

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
				LoggerFromContext(ctx).WarnContext(ctx, "failed to marshal server message", "error", err)
				continue
			}

			writeCtx, writeCancel := context.WithTimeout(ctx, timeout)

			err = conn.Write(writeCtx, websocket.MessageText, data)
			writeCancel()

			if err != nil {
				return fmt.Errorf("write: %w", err)
			}

			// Metrics: message sent
			if r.metrics != nil {
				r.metrics.MessagesSent.WithLabelValues(string(msg.Type)).Inc()
			}

		case <-tickerC:
			pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
			err := conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				return fmt.Errorf("ping timeout: %w", err)
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

// serveWelcome responds with a simple welcome message for non-WebSocket requests.
// If RelayInfo.Name is set, it returns the name. Otherwise, returns empty 200 OK.
func (r *Relay) serveWelcome(w http.ResponseWriter, req *http.Request) {
	if r.info != nil && r.info.Name != "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(r.info.Name))
		return
	}
	// Empty 200 OK
	w.WriteHeader(http.StatusOK)
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

	data, err := json.Marshal(r.info)
	if err != nil {
		r.logger.WarnContext(req.Context(), "failed to marshal relay info", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Write(data)
}
