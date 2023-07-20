package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog/log"
)

func NIP11HandlerFunc(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	ji := jsoniter.ConfigCompatibleWithStandardLibrary

	nip11, err := ji.Marshal(DefaultNIP11)
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

var DefaultNIP11 *NIP11 = &NIP11{
	Name:          Cfg.NIP11Name,
	Description:   Cfg.NIP11Description,
	Pubkey:        Cfg.NIP11Pubkey,
	Contact:       Cfg.NIP11Contact,
	SupportedNips: func() *[]int { v := []int{1, 18, 25}; return &v }(),
	Software:      func() *string { v := "https://github.com/high-moctane/nostr-mocrelay"; return &v }(),

	Limitation: &NIP11Limitation{
		MaxMessageLength: &Cfg.MaxMessageLength,
		MaxSubscriptions: &Cfg.MaxSubscriptions,
		MaxFilters:       &Cfg.MaxFilters,
		MaxSubIDLength:   &Cfg.MaxSubIDLength,
		AuthRequired:     func() *bool { v := false; return &v }(),
		PaymentRequired:  func() *bool { v := false; return &v }(),
	},
}

type NIP11 struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	Pubkey        *string `json:"pubkey,omitempty"`
	Contact       *string `json:"contact,omitempty"`
	SupportedNips *[]int  `json:"supported_nips,omitempty"`
	Software      *string `json:"software,omitempty"`
	Version       *string `json:"version,omitempty"`

	Limitation     *NIP11Limitation       `json:"limitation,omitempty"`
	Retention      *[]NIP11EventRetention `json:"retention,omitempty"`
	RelayCountries *[]string              `json:"relay_countries,omitempty"`
	LanguageTags   *[]string              `json:"language_tags,omitempty"`
	Tags           *[]string              `json:"tags,omitempty"`
	PostingPolicy  *string                `json:"posting_policy,omitempty"`
	PaymentsURL    *string                `json:"payments_url,omitempty"`
	Fees           *NIP11Fees             `json:"fees,omitempty"`
	Icon           *string                `json:"icon,omitempty"`
}

type NIP11Limitation struct {
	MaxMessageLength *int  `json:"max_message_length,omitempty"`
	MaxSubscriptions *int  `json:"max_subscriptions,omitempty"`
	MaxFilters       *int  `json:"max_filters,omitempty"`
	MaxLimit         *int  `json:"max_limit,omitempty"`
	MaxSubIDLength   *int  `json:"max_subid_length,omitempty"`
	MinPrefix        *int  `json:"min_prefix,omitempty"`
	MaxEventTags     *int  `json:"max_event_tags,omitempty"`
	MaxContentLength *int  `json:"max_content_length,omitempty"`
	MinPoWDifficulty *int  `json:"min_pow_difficulty,omitempty"`
	AuthRequired     *bool `json:"auth_required,omitempty"`
	PaymentRequired  *bool `json:"payment_required,omitempty"`
}

type NIP11EventRetention struct {
	Kinds *[]NIP11KindRange `json:"kinds,omitempty"`
	Time  *int              `json:"time,omitempty"`
	Count *int              `json:"count,omitempty"`
}

type NIP11KindRange struct {
	Start, End int
}

func (kr NIP11KindRange) MarshalJSON() ([]byte, error) {
	if kr.Start == kr.End {
		return []byte(strconv.Itoa(kr.Start)), nil
	}
	res, err := json.Marshal([]int{kr.Start, kr.End})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal NIP11KindRange: %v", err)
	}
	return res, nil
}

func (kr *NIP11KindRange) UnmarshalJSON(b []byte) error {
	var i int
	if err := json.Unmarshal(b, &i); err == nil {
		kr.Start = i
		kr.End = i
		return nil
	}

	var sli []int
	if err := json.Unmarshal(b, &sli); err == nil {
		if len(sli) != 2 {
			return errors.New("invalid NIP11KindRange")
		}
		if sli[0] >= sli[1] {
			return errors.New("invalid NIP11KindRange")
		}
		kr.Start = sli[0]
		kr.End = sli[1]
		return nil
	}

	return errors.New("invalid NIP11KindRange")
}

type NIP11Fees struct {
	Admission    *[]NIP11FeeDetail `json:"admission,omitempty"`
	Subscription *[]NIP11FeeDetail `json:"subscription,omitempty"`
	Publication  *[]NIP11FeeDetail `json:"publication,omitempty"`
}

type NIP11FeeDetail struct {
	Amount *int              `json:"amount,omitempty"`
	Unit   *string           `json:"unit,omitempty"`
	Period *int              `json:"period,omitempty"`
	Kinds  *[]NIP11KindRange `json:"kinds,omitempty"`
}
