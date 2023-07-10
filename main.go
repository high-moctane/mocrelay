package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/tomasen/realip"
)

const (
	DefaultDBSize         = 10000
	DefaultAddr           = ":80"
	DefaultClientMsgLen   = 1048576
	DefaultPprofAddr      = ":8396"
	DefaultMaxReqSubIDNum = 20
)

var DBSize = flag.Int("db", DefaultDBSize, "in-memory db size")
var Addr = flag.String("addr", DefaultAddr, "relay addr")
var PprofAddr = flag.String("pprof", DefaultPprofAddr, "relay addr")
var MaxClientMesLen = flag.Int("msglen", DefaultClientMsgLen, "max client message length")
var MaxReqSubIDNum = flag.Int("subid", DefaultMaxReqSubIDNum, "max simultaneous sub_id per connection")
var Verbose = flag.Bool("v", false, "enable verbose log")

var DefaultFilters = Filters{&Filter{&FilterJSON{Kinds: &[]int{
	0, 1, 6, 7,
}}}}

var logStdout = log.New(os.Stdout, "I: ", log.Default().Flags())
var logStderr = log.New(os.Stderr, "E: ", log.Default().Flags())

func init() {
	flag.Parse()
	if !*Verbose {
		f, err := os.Create(os.DevNull)
		if err != nil {
			panic(err)
		}
		logStdout = log.New(f, "", 0)
	}
}

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

	go StartMetricsServer()

	router := NewRouter(DefaultFilters, *MaxReqSubIDNum)
	db := NewDB(*DBSize, DefaultFilters)

	mux := http.NewServeMux()

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sigCtx)

		connID := uuid.NewString()

		if r.URL.Path != "/" {
			http.NotFound(w, r)

		} else if r.Header.Get("Upgrade") != "" {
			conn, _, _, err := ws.UpgradeHTTP(r, w)
			if err != nil {
				logStderr.Printf("[%v, %v]: failed to upgrade http: %v", realip.FromRequest(r), connID, err)
				return
			}
			defer conn.Close()

			DoAccessLog(realip.FromRequest(r), connID, AccessLogConnect, "")
			defer DoAccessLog(realip.FromRequest(r), connID, AccessLogDisconnect, "")

			if err := HandleWebsocket(r.Context(), r, connID, conn, router, db); err != nil {
				logStderr.Printf("[%v, %v]: websocket error: %v", realip.FromRequest(r), connID, err)
			}

		} else if r.Header.Get("Accept") == "application/nostr+json" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "*")
			w.Header().Set("Access-Control-Allow-Methods", "HEAD,OPTIONS,GET")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			if err := HandleNip11(ctx, w, r, connID); err != nil {
				logStderr.Printf("[%v, %v]: failed to serve nip11: %v", realip.FromRequest(r), connID, err)
				return
			}

		} else {
			w.Write([]byte("Welcome to mocrelay (｀･ω･´) !"))
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
