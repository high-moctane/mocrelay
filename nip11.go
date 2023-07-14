package main

import (
	"net/http"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
)

func Nip11HandlerFunc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	nip11, err := ji.Marshal(DefaultNip11)
	if err != nil {
		log.Ctx(ctx).Panic().Err(err).Msg("invalid nip11 json")
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "HEAD,OPTIONS,GET")
	if _, err := w.Write(nip11); err != nil {
		log.Ctx(ctx).Info().Err(err).Msg("failed to send nip11")
	}
}

var DefaultNip11 *Nip11 = &Nip11{
	Name:          "mocrelay",
	Description:   "high-moctane nostr relay. By using this service, you agree that we are not liable for any damages or responsibilities.",
	Pubkey:        "dbf0becf24bf8dd7d779d7fb547e6112964ff042b77a42cc2d8488636eed9f5e",
	Contact:       "mailto:high.moctane@moctane.com",
	SupportedNips: []int{1, 18, 25},
	Software:      "https://github.com/high-moctane/nostr-mocrelay",
}

type Nip11 struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Pubkey        string `json:"pubkey,omitempty"`
	Contact       string `json:"contact,omitempty"`
	SupportedNips []int  `json:"supported_nips,omitempty"`
	Software      string `json:"software,omitempty"`
	Version       string `json:"version,omitempty"`
}
