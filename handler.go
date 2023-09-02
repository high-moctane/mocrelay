package mocrelay

import (
	"net/http"

	"github.com/high-moctane/mocrelay/nostr"
)

type Handler interface {
	Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error
}

type HandlerFunc func(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error

func (f HandlerFunc) Handle(r *http.Request, recv <-chan nostr.ClientMsg, send chan<- nostr.ServerMsg) error {
	return f(r, recv, send)
}
