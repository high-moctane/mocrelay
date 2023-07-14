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
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tomasen/realip"
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

var ConnSema = make(chan struct{}, DefaultMaxConnections)

func init() {
	testing.Init()
	flag.Parse()
}

func main() {
	if err := Run(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("server terminated with error")
	}
}

func Run(ctx context.Context) error {
	ctx = InitLogger(ctx)

	log.Ctx(ctx).Info().Msg("server start")

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt, os.Kill, syscall.SIGPIPE)
	defer stop()

	go StartMetricsServer()

	mux := http.NewServeMux()

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sigCtx)

		realIP := realip.FromRequest(r)
		connID := uuid.NewString()

		ctx := log.Ctx(ctx).With().Str("addr", realIP).Str("conn_id", connID).Logger().WithContext(ctx)

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
				log.Ctx(ctx).Error().Err(err).Msg("failed to upgrade HTTP")
				return
			}
			defer conn.Close()

			log.Ctx(ctx).Info().Msg("connect websocket")
			defer log.Ctx(ctx).Info().Msg("disconnect websocket")

			wreq := NewWebsocketRequest(r, conn, connID)

			handler := DefaultRelay.NewHandler()

			if err := handler.Serve(r.Context(), wreq); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("websocket error")
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
				log.Ctx(ctx).Error().Err(err).Msg("failed to serve nip11")
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

	log.Ctx(ctx).Info().Msg("server stop")
	return nil
}

func InitLogger(ctx context.Context) context.Context {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	ctx = log.With().Logger().WithContext(ctx)
	return ctx
}
