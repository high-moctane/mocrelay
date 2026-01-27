//go:build goexperiment.jsonv2

package mocrelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRelay_Shutdown(t *testing.T) {
	t.Run("shutdown closes active connections", func(t *testing.T) {
		relay := NewRelay(NewNopHandler())
		server := httptest.NewServer(relay)
		defer server.Close()

		// Connect via WebSocket
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		// Start reading in background (will block until connection closes)
		readDone := make(chan error, 1)
		go func() {
			_, _, err := conn.Read(context.Background())
			readDone <- err
		}()

		// Give the connection time to establish
		time.Sleep(50 * time.Millisecond)

		// Shutdown the relay
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()

		if err := relay.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}

		// Verify that the read finished (connection was closed)
		select {
		case <-readDone:
			// Connection was closed as expected
		case <-time.After(time.Second):
			t.Fatal("connection was not closed after shutdown")
		}
	})

	t.Run("shutdown rejects new connections", func(t *testing.T) {
		relay := NewRelay(NewNopHandler())
		server := httptest.NewServer(relay)
		defer server.Close()

		// Shutdown the relay first
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()

		if err := relay.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}

		// Try to connect - should get 503
		resp, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", resp.StatusCode)
		}
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		relay := NewRelay(NewNopHandler())
		server := httptest.NewServer(relay)
		defer server.Close()

		// Connect via WebSocket
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		// Give the connection time to establish
		time.Sleep(50 * time.Millisecond)

		// Shutdown with very short timeout (but connection should close quickly)
		// NopHandler respects ctx cancellation, so this should succeed
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()

		if err := relay.Shutdown(shutdownCtx); err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
	})

	t.Run("connection cleanup removes from map", func(t *testing.T) {
		relay := NewRelay(NewNopHandler())
		server := httptest.NewServer(relay)
		defer server.Close()

		// Connect and immediately close
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		conn, _, err := websocket.Dial(ctx, "ws"+server.URL[4:], nil)
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}

		// Close the connection
		conn.Close(websocket.StatusNormalClosure, "")

		// Wait for cleanup
		time.Sleep(100 * time.Millisecond)

		// Check that cancels map is empty
		relay.mu.Lock()
		numCancels := len(relay.cancels)
		relay.mu.Unlock()

		if numCancels != 0 {
			t.Errorf("expected 0 cancels, got %d", numCancels)
		}
	})
}
