// Nop relay example.
//
// This is the simplest possible relay: it accepts all events (OK true)
// and returns EOSE immediately for all subscriptions.
// No events are stored or relayed.
//
// Usage:
//
//	go run ./cmd/examples/nop
package main

import (
	"log"
	"net/http"

	"github.com/high-moctane/mocrelay"
)

func main() { log.Fatal(http.ListenAndServe(":7447", mocrelay.NewRelay(mocrelay.NewNopHandler()))) }
