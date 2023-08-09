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

var ConfigPath = flag.String("config", "", "config file path")
var Cfg = DefaultConfig

var DefaultFilters = Filters{&Filter{&FilterJSON{Kinds: &[]int{
	0, 1, 6, 7,
}}}}

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
	signal.Ignore()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	InitConfig(ctx)
	ctx = InitZerolog(ctx)

	log.Ctx(ctx).Info().Msg("mocrelay start (｀･ω･´)")
	defer log.Ctx(ctx).Info().Msg("mocrelay stop (｀･ω･´)")

	go StartMetricsServer()
	if err := RunRelayServer(ctx); err != nil {
		return fmt.Errorf("an error occurred on mocrelay: %w", err)
	}

	return nil
}

func InitZerolog(ctx context.Context) context.Context {
	log.Logger = log.Output(os.Stdout).With().Logger()
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro

	switch Cfg.Env {
	case ConfigEnvDev:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case ConfigEnvPrd:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	return log.With().Logger().WithContext(ctx)
}

func InitConfig(ctx context.Context) {
	if *ConfigPath == "" {
		return
	}

	cfg, err := NewConfigFromFilePath(*ConfigPath)
	if err != nil {
		log.Ctx(ctx).Panic().Err(err).Str("filepath", *ConfigPath).Msg("invalid config")
	}
	Cfg = cfg
}
