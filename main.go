package main

import (
	"context"
	"flag"
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
	DefaultDBSize       = 10000
	DefaultAddr         = ":80"
	DefaultClientMsgLen = 1048576
)

var DBSize = flag.Int("db", DefaultDBSize, "in-memory db size")
var Addr = flag.String("addr", DefaultAddr, "relay addr")
var MaxClientMesLen = flag.Int("msglen", DefaultClientMsgLen, "max client message length")

var DefaultFilters = Filters{&Filter{&FilterJSON{Kinds: &[]int{
	0, 1, 6, 7,
}}}}

var logStdout = log.New(os.Stdout, "I: ", log.Default().Flags())
var logStderr = log.New(os.Stderr, "E: ", log.Default().Flags())

func main() {
	logStdout.Printf("server start")
	if err := Run(context.Background()); err != nil {
		logStderr.Fatalf("server terminated with error: %v", err)
	}
	logStdout.Printf("server stop")
}

func Run(ctx context.Context) error {
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt, os.Kill, syscall.SIGPIPE)
	defer stop()

	flag.Parse()

	router := NewRouter(DefaultFilters)
	db := NewDB(*DBSize, DefaultFilters)

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

			logStdout.Printf("[%v]: connect websocket", connID)
			defer logStdout.Printf("[%v]: disconnect websocket", connID)

			if err := HandleWebsocket(r.Context(), r, connID, conn, router, db); err != nil {
				log.Printf("[%v]: websocket error: %v", connID, err)
			}
		}
	}))

	srv := &http.Server{
		Addr:    *Addr,
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
