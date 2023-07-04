package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gobwas/ws/wsutil"
	"github.com/google/uuid"
)

func HandleWebsocket(ctx context.Context, req *http.Request, conn net.Conn, router *Router) {
	sender := make(chan *Event)
	defer close(sender)

	connID := uuid.NewString()
	defer router.Delete(connID)

	for {
		payload, err := wsutil.ReadClientText(conn)
		if err != nil {
			log.Println(err)
			return
		}

		if !utf8.Valid(payload) {
			log.Printf("[%v]: payload is not utf8: %v", connID, payload)
			continue
		}

		jsonMsg, err := ParseClientMsgJSON(string(payload))
		if err != nil {
			log.Printf("[%v]: received invalid msg: %v", connID, err)
			continue
		}

		switch msg := jsonMsg.(type) {
		case ClientReqMsgJSON:
			filters := NewFiltersFromFilterJSONs(msg.FilterJSONs)
			router.Subscribe(connID, msg.SubscriptionID, filters, sender)
		case ClientCloseMsgJSON:
			if err := router.Close(connID, msg.SubscriptionID); err != nil {
				log.Printf("[%v]: cannot close conn %v", connID, msg.SubscriptionID)
				continue
			}
		case ClientEventMsgJSON:
			ok, err := msg.EventJSON.Verify()
			if err != nil {
				log.Printf("[%v]: failed to verify event json: %v", connID, msg)
				continue
			}
			if !ok {
				log.Printf("[%v]: invalid signature: %v", connID, msg)
				continue
			}

			event := &Event{msg.EventJSON, time.Now()}

			if err := router.Publish(event); err != nil {
				log.Printf("[%v]: failed to publish event: %v", connID, event)
				continue
			}
		}
	}
}

func wsReceiver(ctx context.Context, req *http.Request, conn net.Conn, router *Router, receiver chan<- []byte) {
	for {
		msg, err := wsutil.ReadClientText(conn)
		if err != nil {
			log.Println(err)
		}
		receiver <- msg
	}
}

func wsSender(ctx context.Context, req *http.Request, conn net.Conn, router *Router, sender <-chan []byte) {
	for msg := range sender {
		if err := wsutil.WriteServerText(conn, msg); err != nil {
			log.Println(err)
		}
	}
}
