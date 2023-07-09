package main

import (
	"context"
	"fmt"
	"net/http"

	jsoniter "github.com/json-iterator/go"
)

func HandleNip11(ctx context.Context, w http.ResponseWriter, r *http.Request, connID string) error {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	nip11, err := ji.Marshal(DefaultNip11)
	if err != nil {
		panic(fmt.Sprintf("invalid nip11 json: %v", err))
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err := w.Write(nip11); err != nil {
		return fmt.Errorf("failed to send nip11: %w", err)
	}
	return nil
}

var DefaultNip11 *Nip11 = &Nip11{
	Name:          "mocrelay",
	Description:   "high-moctane nostr relay",
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
