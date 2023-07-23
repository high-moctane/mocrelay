package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func RunRelayServer(ctx context.Context) error {
	srv := NewServerWithCtx(ctx)

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		StopServerOnCtxDone(ctx, srv)
	}()

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("an error occurred on serving http: %w", err)
	}

	wg.Wait()
	return nil
}

func NewServerWithCtx(ctx context.Context) *http.Server {
	mux := http.NewServeMux()

	mux.Handle("/", BuildHandler(
		RootHandler(),
		WithContextMiddlewareBuilder(ctx),
		AddRequestDataMiddleware,
		ZerologRequestCtxMiddleware,
		RejectTooManyRequestMiddleware,
	))

	return &http.Server{
		Addr:    Cfg.Addr,
		Handler: mux,
	}
}

func StopServerOnCtxDone(ctx context.Context, srv *http.Server) error {
	<-ctx.Done()

	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(c); err != nil {
		return fmt.Errorf("failed to stop server gracefully")
	}

	return nil
}

func BuildHandler(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	if len(middlewares) == 0 {
		return handler
	}
	return middlewares[0](BuildHandler(handler, middlewares[1:]...))
}

func RootHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFoundHandler().ServeHTTP(w, r)

		} else if r.Header.Get("Upgrade") != "" {
			RelayAccessHandlerFunc(w, r)

		} else if r.Header.Get("Accept") == "application/nostr+json" {
			NIP11HandlerFunc(w, r)

		} else {
			DefaultPageHandlerFunc(w, r)
		}
	})
}
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func DefaultPageHandlerFunc(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(Cfg.TopPageMessage))
}

func WithContextMiddlewareBuilder(ctx context.Context) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AddRequestDataMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = SetCtxRealIPFronRequest(ctx, r)
		ctx = SetCtxConnID(ctx, uuid.NewString())
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func ZerologRequestCtxMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ctx = log.
			Ctx(ctx).
			With().
			Str("addr", GetCtxRealIP(ctx)).
			Str("conn_id", GetCtxConnID(ctx)).
			Logger().
			WithContext(ctx)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RejectTooManyRequestMiddleware(h http.Handler) http.Handler {
	sema := make(chan struct{}, Cfg.MaxConnections)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sema <- struct{}{}:
			defer func() { <-sema }()
			h.ServeHTTP(w, r)
		default:
			RejectTooManyRequestHandler(w, r)
		}
	})
}

func RejectTooManyRequestHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusTooManyRequests)
}
