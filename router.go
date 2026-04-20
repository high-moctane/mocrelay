package mocrelay

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Router manages client connections and subscriptions.
// It routes events to clients based on their subscription filters.
type Router struct {
	mu sync.RWMutex

	// nextConnID is used to generate unique connection IDs.
	nextConnID uint64

	// connections maps connection ID to connection info.
	connections map[string]*routerConnection

	metrics *routerMetrics
}

// routerConnection represents a single client connection.
type routerConnection struct {
	sendCh chan<- *ServerMsg

	// subscriptions maps subscription ID to filters.
	// Subscription ID is unique within a connection (provided by client).
	subscriptions map[string][]*ReqFilter
}

// RouterOptions configures Router behavior.
// All fields are optional; the zero value gives sensible defaults.
type RouterOptions struct {
	// Registerer is the Prometheus registry that Router's internal metrics
	// are registered with (mocrelay_router_messages_dropped_total and
	// mocrelay_router_subscriptions_current). If nil, no metrics are
	// collected.
	Registerer prometheus.Registerer
}

// NewRouter creates a new Router.
// opts can be nil for default options.
func NewRouter(opts *RouterOptions) *Router {
	if opts == nil {
		opts = &RouterOptions{}
	}
	return &Router{
		connections: make(map[string]*routerConnection),
		metrics:     newRouterMetrics(opts.Registerer),
	}
}

// Register registers a new connection and returns the connection ID.
// The sendCh is used to send messages to this connection.
func (r *Router) Register(sendCh chan<- *ServerMsg) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextConnID++
	connID := formatConnID(r.nextConnID)

	r.connections[connID] = &routerConnection{
		sendCh:        sendCh,
		subscriptions: make(map[string][]*ReqFilter),
	}

	return connID
}

// Unregister removes a connection and all its subscriptions.
func (r *Router) Unregister(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, ok := r.connections[connID]
	if !ok {
		return
	}
	if r.metrics != nil && len(conn.subscriptions) > 0 {
		r.metrics.SubscriptionsCurrent.Sub(float64(len(conn.subscriptions)))
	}
	delete(r.connections, connID)
}

// Subscribe registers a subscription for a connection.
// If the subscription ID already exists, it will be replaced.
func (r *Router) Subscribe(connID, subID string, filters []*ReqFilter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, ok := r.connections[connID]
	if !ok {
		return
	}

	// Only increment the gauge on a genuinely new subID; a client
	// replacing an existing subscription (NIP-01 allows this without an
	// intervening CLOSE) must not cause drift.
	_, existed := conn.subscriptions[subID]
	conn.subscriptions[subID] = filters
	if r.metrics != nil && !existed {
		r.metrics.SubscriptionsCurrent.Inc()
	}
}

// Unsubscribe removes a subscription from a connection.
func (r *Router) Unsubscribe(connID, subID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conn, ok := r.connections[connID]
	if !ok {
		return
	}

	if _, existed := conn.subscriptions[subID]; existed {
		delete(conn.subscriptions, subID)
		if r.metrics != nil {
			r.metrics.SubscriptionsCurrent.Dec()
		}
	}
}

// Broadcast sends an event to all matching subscriptions.
// This is best-effort: if a connection's channel is full, the message is dropped.
func (r *Router) Broadcast(event *Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, conn := range r.connections {
		for subID, filters := range conn.subscriptions {
			if matchesAny(filters, event) {
				// Best-effort send: drop if channel is full
				select {
				case conn.sendCh <- NewServerEventMsg(subID, event):
				default:
					// Channel full, drop message
					if r.metrics != nil {
						r.metrics.MessagesDropped.Inc()
					}
				}
				// Don't break: same event could match multiple subscriptions
				// But we only send once per subscription
			}
		}
	}
}

// matchesAny returns true if the event matches any of the filters.
func matchesAny(filters []*ReqFilter, event *Event) bool {
	for _, f := range filters {
		if f.Match(event) {
			return true
		}
	}
	return false
}

// formatConnID formats a connection ID from a uint64.
func formatConnID(id uint64) string {
	// Simple string conversion for now
	// Could use hex or base64 for shorter strings
	return "conn-" + uitoa(id)
}

// uitoa converts a uint64 to a string without importing strconv.
func uitoa(val uint64) string {
	if val == 0 {
		return "0"
	}
	var buf [20]byte // max uint64 is 20 digits
	i := len(buf)
	for val > 0 {
		i--
		buf[i] = byte('0' + val%10)
		val /= 10
	}
	return string(buf[i:])
}
