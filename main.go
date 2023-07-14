package main

import (
	"context"
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
	ctx = InitZerolog(ctx)

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt, os.Kill, syscall.SIGPIPE)
	defer stop()

	log.Ctx(ctx).Info().Msg("mocrelay start (｀･ω･´)")
	defer log.Ctx(ctx).Info().Msg("mocrelay stop (｀･ω･´)")

	go StartMetricsServer()
	if err := RunRelayServer(sigCtx); err != nil {
		return fmt.Errorf("an error occurred on mocrelay: %w", err)
	}

	return nil
}

func InitZerolog(ctx context.Context) context.Context {
	log.Logger = log.Output(os.Stdout).With().Logger()
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	return log.With().Logger().WithContext(ctx)
}
