package main

import (
	"time"

	jsoniter "github.com/json-iterator/go"
)

func NewEvent(json *EventJSON, receivedAt time.Time) *Event {
	return &Event{
		EventJSON:  json,
		ReceivedAt: receivedAt,
	}
}

type Event struct {
	*EventJSON
	ReceivedAt time.Time
}

func (e *Event) ValidCreatedAt() bool {
	sub := time.Until(e.CreatedAtToTime())
	// TODO(high-moctane) no magic number
	return -10*time.Minute <= sub && sub <= 5*time.Minute
}

func (e *Event) MarshalJSON() ([]byte, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary
	return ji.Marshal(e.EventJSON)
}
