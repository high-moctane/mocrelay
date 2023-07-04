package main

import (
	"fmt"
	"sync"
)

type Router struct {
	mu          *sync.RWMutex
	subscribers []*subscriber
}

func (rtr *Router) Subscribe(connID, subID string, recv chan<- *Event, filters Filters) {
	newSubscr := &subscriber{
		ConnectionID: connID,
		SubscriptID:  subID,
		RecvCh:       recv,
		Filters:      filters,
	}

	rtr.mu.Lock()
	defer rtr.mu.Unlock()

	for i, subscr := range rtr.subscribers {
		if subscr.ConnectionID == connID && subscr.SubscriptID == subID {
			rtr.subscribers[i] = newSubscr
			return
		}
	}
	rtr.subscribers = append(rtr.subscribers, newSubscr)
}

func (rtr *Router) Close(connID, subID string) error {
	rtr.mu.Lock()
	defer rtr.mu.Unlock()

	found := false
	for i, subscr := range rtr.subscribers {
		if subscr.ConnectionID == connID && subscr.SubscriptID == subID {
			rtr.subscribers[i], rtr.subscribers[len(rtr.subscribers)-1] =
				rtr.subscribers[len(rtr.subscribers)-1], rtr.subscribers[i]
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("subscr not found (connID=%q, subID=%q)", connID, subID)
	}

	rtr.subscribers = rtr.subscribers[:len(rtr.subscribers)-1]

	return nil
}

func (rtr *Router) Delete(connID string) {
	rtr.mu.Lock()
	defer rtr.mu.Unlock()

	for i := len(rtr.subscribers) - 1; i >= 0; i-- {
		if rtr.subscribers[i].ConnectionID == connID {
			rtr.subscribers[i], rtr.subscribers[len(rtr.subscribers)-1] =
				rtr.subscribers[len(rtr.subscribers)-1], rtr.subscribers[i]
		}
		rtr.subscribers = rtr.subscribers[:len(rtr.subscribers)-1]
	}
}

func (rtr *Router) Publish(event *Event) error {
	rtr.mu.RLock()
	defer rtr.mu.RUnlock()

	for _, subscr := range rtr.subscribers {
		subscr.Receive(event)
	}

	return nil
}

type subscriber struct {
	ConnectionID string
	SubscriptID  string
	RecvCh       chan<- *Event
	Filters      Filters
}

func (sb *subscriber) Receive(event *Event) bool {
	if sb.Filters.Match(event) {
		select {
		case sb.RecvCh <- event:
			return true
		default:
			return false
		}
	}

	return false
}
