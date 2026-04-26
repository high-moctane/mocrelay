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
	"github.com/prometheus/client_golang/prometheus"
)

// Relay wraps a Handler to serve it over HTTP/WebSocket.
// It implements http.Handler.
type Relay struct {
	handler          Handler
	logger           *slog.Logger
	maxMessageLength int64
	pingInterval     time.Duration
	pingTimeout      time.Duration
	readRate         float64
	readBurst        int
	info             *RelayInfo
	metrics          *relayMetrics
	rejectionMetrics *rejectionMetrics
	authMetrics      *authMetrics
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

	// ReadRate is the maximum messages-per-second the readLoop will pull
	// off the WebSocket on a single connection. When the per-connection
	// budget is exhausted the readLoop STALLS -- conn.Read is simply not
	// invoked until a token refills -- so back-pressure propagates to the
	// client through the TCP receive window. The relay never spends parse
	// or signature-verification CPU on throttled traffic.
	//
	// This is the readLoop-level total cap and is independent of the
	// per-message-type rate limit middlewares (Event/Req/Count/Auth),
	// which run after parse and let operators tune category-specific
	// budgets. Both layers compose: readLoop acts first as a coarse
	// floodgate, middlewares then refine.
	//
	// Zero or negative disables the limit (default).
	ReadRate float64

	// ReadBurst is the bucket capacity for the readLoop rate limit. Must
	// be >= 1 if ReadRate > 0; the constructor panics on misconfiguration
	// rather than silently degrading. The bucket starts full at OnStart
	// so a freshly-connected client can burst up to ReadBurst messages
	// immediately, then is throttled to ReadRate long-term.
	//
	// Ignored when ReadRate <= 0.
	ReadBurst int

	// Info is the NIP-11 Relay Information Document.
	// If set, the relay will respond to HTTP requests with
	// Accept: application/nostr+json header.
	Info *RelayInfo

	// Registerer is the Prometheus registry that mocrelay's internal
	// metrics are registered with. Setting this single field turns on every
	// metric mocrelay exports from the Relay's own scope:
	//
	//   - Connection / message / event counters on the Relay itself
	//     (mocrelay_connections_*, mocrelay_messages_*, mocrelay_events_received_*,
	//     mocrelay_ws_parse_errors_*, mocrelay_ws_write_errors_*,
	//     mocrelay_ws_write_duration_seconds).
	//   - The unified rejection counter
	//     (mocrelay_rejections_total{middleware, reason}), automatically
	//     installed into the request context so that every middleware's
	//     logRejection call increments it.
	//   - Auth middleware counters (mocrelay_auth_*,
	//     mocrelay_auth_authenticated_connections_current), also installed
	//     into the request context and read by the auth middleware.
	//
	// Metrics owned by types that compose *into* Relay (Router, Storage,
	// CompositeStorage, MergeHandler, MetricsStorage) are registered from
	// their own constructors; pass the same Registerer to each of those
	// Options structs (typically prometheus.DefaultRegisterer) to collect
	// everything under one registry.
	//
	// If nil, no metrics are registered from this Relay and all internal
	// instrumentation no-ops.
	Registerer prometheus.Registerer

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

	if opts.ReadRate > 0 && opts.ReadBurst < 1 {
		panic("mocrelay: RelayOptions.ReadBurst must be >= 1 when ReadRate > 0")
	}

	return &Relay{
		handler:          handler,
		logger:           logger,
		maxMessageLength: maxMessageLength,
		pingInterval:     pingInterval,
		pingTimeout:      pingTimeout,
		readRate:         opts.ReadRate,
		readBurst:        opts.ReadBurst,
		info:             opts.Info,
		metrics:          newRelayMetrics(opts.Registerer),
		rejectionMetrics: newRejectionMetrics(opts.Registerer),
		authMetrics:      newAuthMetrics(opts.Registerer),
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
		ctx = contextWithRejectionMetrics(ctx, r.rejectionMetrics)
	}
	if r.authMetrics != nil {
		ctx = contextWithAuthMetrics(ctx, r.authMetrics)
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
	// Per-connection token bucket for the readLoop-level rate limit.
	// Disabled when ReadRate <= 0; readStall short-circuits on nil bucket.
	var bucket *tokenBucket
	if r.readRate > 0 {
		bucket = newTokenBucket(r.readRate, r.readBurst, time.Now())
	}

	var onStall func()
	if r.metrics != nil {
		onStall = r.metrics.ReadStallsTotal.Inc
	}

	for {
		// Stall before conn.Read so a throttled client never costs us a
		// frame read, parse, or signature verify. Back-pressure rides on
		// the TCP receive window once the OS-level WebSocket buffer fills.
		if err := readStall(ctx, bucket, onStall); err != nil {
			return err
		}

		typ, payload, err := conn.Read(ctx)
		if err != nil {
			return err
		}

		// Must be text message
		if typ != websocket.MessageText {
			LoggerFromContext(ctx).WarnContext(ctx, "received binary message")
			if r.metrics != nil {
				r.metrics.WSParseErrors.WithLabelValues("binary_message").Inc()
			}
			r.sendNotice(ctx, send, "binary message not allowed")
			continue
		}

		// Must be valid UTF-8
		if !utf8.Valid(payload) {
			LoggerFromContext(ctx).WarnContext(ctx, "received invalid UTF-8")
			if r.metrics != nil {
				r.metrics.WSParseErrors.WithLabelValues("invalid_utf8").Inc()
			}
			r.sendNotice(ctx, send, "invalid UTF-8")
			continue
		}

		// Parse client message
		msg, err := ParseClientMsg(payload)
		if err != nil {
			LoggerFromContext(ctx).WarnContext(ctx, "failed to parse client message", "error", err)
			if r.metrics != nil {
				r.metrics.WSParseErrors.WithLabelValues("parse_error").Inc()
			}
			r.sendNotice(ctx, send, "invalid message format")
			continue
		}

		// Verify event signature if EVENT message
		if msg.Type == MsgTypeEvent && msg.Event != nil {
			valid, err := msg.Event.Verify()
			if err != nil {
				LoggerFromContext(ctx).WarnContext(ctx, "failed to verify event", "error", err)
				if r.metrics != nil {
					r.metrics.WSParseErrors.WithLabelValues("verify_error").Inc()
				}
				r.sendNotice(ctx, send, "verification error")
				continue
			}
			if !valid {
				LoggerFromContext(ctx).WarnContext(ctx, "invalid event signature", "id", msg.Event.ID)
				if r.metrics != nil {
					r.metrics.WSParseErrors.WithLabelValues("invalid_signature").Inc()
				}
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
				if r.metrics != nil {
					r.metrics.WSWriteErrors.WithLabelValues("marshal").Inc()
				}
				continue
			}

			writeCtx, writeCancel := context.WithTimeout(ctx, timeout)

			writeStart := time.Now()
			err = conn.Write(writeCtx, websocket.MessageText, data)
			writeCancel()
			if r.metrics != nil {
				r.metrics.WSWriteDuration.Observe(time.Since(writeStart).Seconds())
			}

			if err != nil {
				if r.metrics != nil {
					reason := "other"
					if errors.Is(err, context.DeadlineExceeded) {
						reason = "timeout"
					}
					r.metrics.WSWriteErrors.WithLabelValues(reason).Inc()
				}
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
				if r.metrics != nil {
					r.metrics.WSWriteErrors.WithLabelValues("ping_timeout").Inc()
				}
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
