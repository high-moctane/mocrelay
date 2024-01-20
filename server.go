package mocrelay

import (
	"context"
	"io"
	"log/slog"
	"net/http"
)

type ServeMux struct {
	Relay   *Relay
	NIP11   *NIP11
	Default http.Handler
	Logger  *slog.Logger
}

func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "" {
		mux.logInfo(r.Context(), "got relay access")
		mux.Relay.ServeHTTP(w, r)

	} else if r.Header.Get("Accept") == "application/nostr+json" {
		mux.logInfo(r.Context(), "got nip11 access")
		if mux.NIP11 == nil {
			io.WriteString(w, "{}")
		} else {
			mux.NIP11.ServeHTTP(w, r)
		}

	} else {
		mux.logInfo(r.Context(), "got default access")
		if mux.Default == nil {
			io.WriteString(w, "Hello Mocrelay (｀･ω･´)！")
		} else {
			mux.Default.ServeHTTP(w, r)
		}
	}
}

func (mux *ServeMux) logInfo(ctx context.Context, msg string, args ...any) {
	if mux.Logger == nil {
		return
	}
	mux.Logger.InfoContext(ctx, msg, args...)
}
