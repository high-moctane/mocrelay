package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/google/uuid"
	"github.com/tomasen/realip"
	"go.uber.org/zap"
)

const (
	DefaultCacheSize      = 10000
	DefaultAddr           = ":80"
	DefaultClientMsgLen   = 1048576
	DefaultPprofAddr      = ":8396"
	DefaultMaxReqSubIDNum = 20
	DefaultMaxConnections = 10000
)

var CacheSize = flag.Int("cache", DefaultCacheSize, "in-memory cache size")
var Addr = flag.String("addr", DefaultAddr, "relay addr")
var PprofAddr = flag.String("pprof", DefaultPprofAddr, "relay addr")
var MaxClientMesLen = flag.Int("msglen", DefaultClientMsgLen, "max client message length")
var MaxReqSubIDNum = flag.Int("subid", DefaultMaxReqSubIDNum, "max simultaneous sub_id per connection")

var DefaultFilters = Filters{&Filter{&FilterJSON{Kinds: &[]int{
	0, 1, 6, 7,
}}}}

var logger *zap.SugaredLogger

var ConnSema = make(chan struct{}, DefaultMaxConnections)

func init() {
	testing.Init()
	flag.Parse()
}

func main() {
	if err := Run(context.Background()); err != nil {
		logger.Fatal("server terminated with error", zap.Error(err))
	}

}

func Run(ctx context.Context) error {
	logger = zap.Must(zap.NewDevelopment()).Sugar()
	defer logger.Sync()

	logger.Info("server starting")

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt, os.Kill, syscall.SIGPIPE)
	defer stop()

	go StartMetricsServer()

	relay := new(Relay)
	router := NewRouter(DefaultFilters, *MaxReqSubIDNum)
	cache := NewCache(*CacheSize, DefaultFilters)

	mux := http.NewServeMux()

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sigCtx)

		connID := uuid.NewString()

		select {
		case ConnSema <- struct{}{}:
			defer func() { <-ConnSema }()
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		if r.URL.Path != "/" {
			http.NotFound(w, r)

		} else if r.Header.Get("Upgrade") != "" {
			conn, _, _, err := ws.UpgradeHTTP(r, w)
			if err != nil {
				logger.Errorw("failed to upgrade http",
					"addr", realip.FromRequest(r),
					"conn_id", connID,
					"error", err)
				return
			}
			defer conn.Close()

			logger.Infow("connect websocket",
				"addr", realip.FromRequest(r),
				"conn", connID)
			defer logger.Infow("disconnect websocket",
				"addr", realip.FromRequest(r),
				"conn", connID)

			if err := relay.HandleWebsocket(r.Context(), r, connID, conn, router, cache); err != nil {
				logger.Errorw("websocket error",
					"addr", realip.FromRequest(r),
					"conn_id", connID,
					"error", err)
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
				logger.Errorw("failed to serve nip11",
					"addr", realip.FromRequest(r),
					"conn_id", connID,
					"error", err)
				return
			}

		} else {
			w.Write([]byte("Welcome to mocrelay (｀･ω･´) !\n\nBy using this service, you agree that we are not liable for any damages or responsibilities.\n"))
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

	if err := srv.ListenAndServe(); err != nil {
		return err
	}

	logger.Info("server stop")
	return nil
}
