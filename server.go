package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gobwas/ws/wsutil"
)

func HandleWebsocket(ctx context.Context, req *http.Request, connID string, conn net.Conn, router *Router) {
	defer func() {
		if err := recover(); err != nil {
			logStderr.Printf("[%v]: paniced: %v", connID, err)
			panic(err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer router.Delete(connID)

	sender := make(chan ServerMsg, 3)

	go wsSender(ctx, req, connID, conn, router, sender)

	if err := wsReceiver(ctx, req, connID, conn, router, sender); err != nil {
		if !errors.Is(err, io.EOF) {
			logStderr.Printf("[%v]: network error: %v", connID, err)
			return
		}
	}
}

func wsReceiver(ctx context.Context, req *http.Request, connID string, conn net.Conn, router *Router, sender chan<- ServerMsg) error {
	for {
		payload, err := wsutil.ReadClientText(conn)
		if err != nil {
			return fmt.Errorf("[%v]: receive error: %w", connID, err)
		}

		if !utf8.Valid(payload) {
			logStderr.Printf("[%v]: payload is not utf8: %v", connID, payload)
			continue
		}

		strMsg := string(payload)
		jsonMsg, err := ParseClientMsgJSON(strMsg)
		if err != nil {
			logStderr.Printf("[%v]: received invalid msg: %v", connID, err)
			continue
		}

		logStdout.Printf("[%v]: recv: %v", connID, strMsg)

		switch msg := jsonMsg.(type) {
		case *ClientReqMsgJSON:
			if err := serveClientReqMsgJSON(connID, router, sender, msg); err != nil {
				logStderr.Printf("[%v]: failed to serve client req msg %v", connID, err)
				continue
			}

		case *ClientCloseMsgJSON:
			if err := serveClientCloseMsgJSON(connID, router, msg); err != nil {
				logStderr.Printf("[%v]: failed to serve client close msg %v", connID, err)
				continue
			}

		case *ClientEventMsgJSON:
			if err := serveClientEventMsgJSON(router, msg); err != nil {
				logStderr.Printf("[%v]: failed to serve client event msg %v", connID, err)
				continue
			}
		}
	}
}

func serveClientReqMsgJSON(connID string, router *Router, sender chan<- ServerMsg, msg *ClientReqMsgJSON) error {
	filters := NewFiltersFromFilterJSONs(msg.FilterJSONs)
	router.Subscribe(connID, msg.SubscriptionID, filters, sender)
	sender <- &ServerEOSEMsg{msg.SubscriptionID}
	return nil
}

func serveClientCloseMsgJSON(connID string, router *Router, msg *ClientCloseMsgJSON) error {
	if err := router.Close(connID, msg.SubscriptionID); err != nil {
		return fmt.Errorf("cannot close conn %v", msg.SubscriptionID)
	}
	return nil
}

func serveClientEventMsgJSON(router *Router, msg *ClientEventMsgJSON) error {
	ok, err := msg.EventJSON.Verify()
	if err != nil {
		return fmt.Errorf("failed to verify event json: %v", msg)

	}
	if !ok {
		return fmt.Errorf("invalid signature: %v", msg)
	}

	event := &Event{msg.EventJSON, time.Now()}

	if err := router.Publish(event); err != nil {
		return fmt.Errorf("failed to publish event: %v", event)
	}
	return nil
}

func wsSender(ctx context.Context, req *http.Request, connID string, conn net.Conn, router *Router, sender <-chan ServerMsg) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-sender:
			jsonMsg, err := msg.MarshalJSON()
			if err != nil {
				logStderr.Printf("[%v]: failed to marshal server msg: %v", connID, msg)
			}

			if err := wsutil.WriteServerText(conn, jsonMsg); err != nil {
				logStderr.Printf("[%v]: failed to write server text: %v", connID, err)
			}

			logStdout.Printf("[%v]: send: %v", connID, string(jsonMsg))
		}
	}
}
