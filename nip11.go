package mocrelay

import (
	"encoding/json"
	"io"
	"net/http"
)

type NIP11 struct {
	Name          string           `json:"name,omitempty"`
	Description   string           `json:"description,omitempty"`
	Pubkey        string           `json:"pubkey,omitempty"`
	Contact       string           `json:"contact,omitempty"`
	SupportedNIPs []int            `json:"supported_nips,omitempty"`
	Software      string           `json:"software,omitempty"`
	Version       string           `json:"version,omitempty"`
	Limitation    *NIP11Limitation `json:"limitation,omitempty"`
	Retention     *NIP11Retention  `json:"retention,omitempty"`
	RelayContries []string         `json:"relay_countries,omitempty"`
	LanguageTags  []string         `json:"language_tags,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	PostingPolicy string           `json:"posting_policy,omitempty"`
	PaymentsURL   string           `json:"payments_url,omitempty"`
	Fees          *NIP11Fees       `json:"fees,omitempty"`
	Icon          string           `json:"icon,omitempty"`
}

type NIP11Limitation struct {
	MaxMessageLength    int   `json:"max_message_length,omitempty"`
	MaxSubscriptions    int   `json:"max_subscriptions,omitempty"`
	MaxFilters          int   `json:"max_filters,omitempty"`
	MaxLimit            int   `json:"max_limit,omitempty"`
	MaxSubIDLength      int   `json:"max_subid_length,omitempty"`
	MaxEventTags        int   `json:"max_event_tags,omitempty"`
	MaxContentLength    int   `json:"max_content_length,omitempty"`
	MinPoWDifficulty    int   `json:"min_pow_difficulty,omitempty"`
	AuthRequired        bool  `json:"auth_required,omitempty"`
	PaymentRequired     bool  `json:"payment_required,omitempty"`
	CreatedAtLowerLimit int64 `json:"created_at_lower_limit,omitempty"`
	CreatedAtUpperLimit int64 `json:"created_at_upper_limit,omitempty"`
}

type NIP11Retention struct {
	// TODO(high-moctane) Impl
}

type NIP11Fees struct {
	// TODO(high-moctane) Impl
}

func (nip11 *NIP11) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") != "application/nostr+json" {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Need an Accept header of application/nostr+json")
		return
	}

	nip11json, err := json.Marshal(nip11)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal Server Error")
		return
	}

	w.Header().Add("Content-Type", "application/nostr+json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Write(nip11json)
}
