package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

func NewRouter(fil Filters, maxSubIDNum int) *Router {
	return &Router{filters: fil, maxSubIDNum: maxSubIDNum}
}

type Router struct {
	mu          sync.RWMutex
	subscribers []*subscriber
	filters     Filters
	maxSubIDNum int
}

func (rtr *Router) Subscribe(connID, subID string, filters Filters, sendFunc func(ServerMsg) bool) error {
	if len(subID) > 64 {
		return fmt.Errorf("too long subscriptsion_id: %v", subID)
	}

	newSubscr := &subscriber{
		ConnectionID: connID,
		SubscriptID:  subID,
		SendFunc:     sendFunc,
		Filters:      filters,
	}

	rtr.mu.Lock()
	defer rtr.mu.Unlock()

	cnt := 0
	for i, subscr := range rtr.subscribers {
		if subscr.ConnectionID == connID {
			cnt++
			if subscr.SubscriptID == subID {
				rtr.subscribers[i] = newSubscr
				return nil
			} else if rtr.maxSubIDNum > 0 && cnt >= rtr.maxSubIDNum {
				return errors.New("too much req")
			}
		}
	}
	rtr.subscribers = append(rtr.subscribers, newSubscr)
	return nil
}

func (rtr *Router) Close(connID, subID string) error {
	found := false

	rtr.mu.Lock()
	defer rtr.mu.Unlock()

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

	rtr.subscribers[len(rtr.subscribers)-1] = nil
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

			rtr.subscribers[len(rtr.subscribers)-1] = nil
			rtr.subscribers = rtr.subscribers[:len(rtr.subscribers)-1]
		}
	}
}

func (rtr *Router) Publish(event *Event) error {
	if !rtr.filters.Match(event) {
		return nil
	}

	start := time.Now()
	defer promRouterPublishTime.WithLabelValues(event).Observe(time.Since(start).Seconds())

	rtr.mu.RLock()
	defer rtr.mu.RUnlock()

	for _, subscr := range rtr.subscribers {
		subscr.TrySend(event)
	}

	return nil
}

type subscriber struct {
	ConnectionID string
	SubscriptID  string
	SendFunc     func(ServerMsg) bool
	Filters      Filters
}

func (sb *subscriber) TrySend(event *Event) bool {
	if sb.Filters.Match(event) {
		return sb.SendFunc(NewServerEventMsg(sb.SubscriptID, event))
	}

	return false
}
