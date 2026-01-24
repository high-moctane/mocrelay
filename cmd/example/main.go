//go:build goexperiment.jsonv2

package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/high-moctane/mocrelay"
)

func main() {
	addr := ":8080"
	if envAddr := os.Getenv("ADDR"); envAddr != "" {
		addr = envAddr
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	handler := mocrelay.NewNopHandler()
	relay := mocrelay.NewRelay(handler)
	relay.Logger = logger

	logger.Info("starting relay", "addr", addr)
	if err := http.ListenAndServe(addr, relay); err != nil {
		log.Fatal(err)
	}
}
