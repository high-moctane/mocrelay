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

	logger *slog.Logger
}

func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx = ctxWithRealIP(ctx, r)
	ctx = ctxWithRequestID(ctx)
	ctx = ctxWithHTTPHeader(ctx, r)
	r = r.WithContext(ctx)

	if mux.Logger != nil {
		mux.logger = slog.New(WithSlogMocrelayHandler(mux.Logger.Handler()))
	}

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
	if mux.logger == nil {
		return
	}
	mux.logger.InfoContext(ctx, msg, args...)
}
