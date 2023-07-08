package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
)

const (
	DBSize = 10000
)

var logStdout = log.New(os.Stdout, "I: ", log.Default().Flags())
var logStderr = log.New(os.Stderr, "E: ", log.Default().Flags())

func main() {
	logStderr.Printf("server start")
	if err := Run(context.Background()); err != nil {
		logStderr.Fatalf("server terminated with error: %v", err)
	}
	logStderr.Printf("server stop")
}

func Run(ctx context.Context) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt, os.Kill, syscall.SIGPIPE)
	defer stop()

	filters := Filters{&Filter{&FilterJSON{Kinds: &[]int{
		0, 1, 6, 7,
	}}}}

	router := NewRouter(filters)
	db := NewDB(DBSize, filters)

	mux := http.NewServeMux()

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sigCtx)

		connID := uuid.NewString()

		switch r.Header.Get("Accept") {
		case "application/nostr+json":
			if err := HandleNip11(ctx, w, r, connID); err != nil {
				logStderr.Printf("[%v]: failed to serve nip11: %v", connID, err)
				return
			}

		default:
			conn, _, _, err := ws.UpgradeHTTP(r, w)
			if err != nil {
				log.Printf("[%v]: failed to upgrade http: %v", connID, err)
				return
			}
			defer conn.Close()

			if err := HandleWebsocket(r.Context(), r, connID, conn, router, db); err != nil {
				log.Printf("[%v]: websocket error: %v", connID, err)
			}
		}
	}))

	srv := &http.Server{
		Addr:    "127.0.0.1:8234",
		Handler: mux,
	}

	go func() {
		<-sigCtx.Done()

		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		srv.Shutdown(ctx)
	}()

	return srv.ListenAndServe()
}
