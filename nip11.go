package mocrelay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"gopkg.in/yaml.v3"
)

type NIP11 struct {
	Name          string           `json:"name,omitempty" yaml:"name,omitempty"`
	Description   string           `json:"description,omitempty" yaml:"description,omitempty"`
	Pubkey        string           `json:"pubkey,omitempty" yaml:"pubkey,omitempty"`
	Contact       string           `json:"contact,omitempty" yaml:"contact,omitempty"`
	SupportedNIPs []int            `json:"supported_nips,omitempty" yaml:"supported_nips,omitempty"`
	Software      string           `json:"software,omitempty" yaml:"software,omitempty"`
	Version       string           `json:"version,omitempty" yaml:"version,omitempty"`
	Limitation    *NIP11Limitation `json:"limitation,omitempty" yaml:"limitation,omitempty"`
	Retention     *NIP11Retention  `json:"retention,omitempty" yaml:"retention,omitempty"`
	RelayContries []string         `json:"relay_countries,omitempty" yaml:"relay_countries,omitempty"`
	LanguageTags  []string         `json:"language_tags,omitempty" yaml:"language_tags,omitempty"`
	Tags          []string         `json:"tags,omitempty" yaml:"tags,omitempty"`
	PostingPolicy string           `json:"posting_policy,omitempty" yaml:"posting_policy,omitempty"`
	PaymentsURL   string           `json:"payments_url,omitempty" yaml:"payments_url,omitempty"`
	Fees          *NIP11Fees       `json:"fees,omitempty" yaml:"fees,omitempty"`
	Icon          string           `json:"icon,omitempty" yaml:"icon,omitempty"`
}

type NIP11Limitation struct {
	MaxMessageLength    int   `json:"max_message_length,omitempty" yaml:"max_message_length,omitempty"`
	MaxSubscriptions    int   `json:"max_subscriptions,omitempty" yaml:"max_subscriptions,omitempty"`
	MaxFilters          int   `json:"max_filters,omitempty" yaml:"max_filters,omitempty"`
	MaxLimit            int   `json:"max_limit,omitempty" yaml:"max_limit,omitempty"`
	MaxSubIDLength      int   `json:"max_subid_length,omitempty" yaml:"max_subid_length,omitempty"`
	MaxEventTags        int   `json:"max_event_tags,omitempty" yaml:"max_event_tags,omitempty"`
	MaxContentLength    int   `json:"max_content_length,omitempty" yaml:"max_content_length,omitempty"`
	MinPoWDifficulty    int   `json:"min_pow_difficulty,omitempty" yaml:"min_pow_difficulty,omitempty"`
	AuthRequired        bool  `json:"auth_required,omitempty" yaml:"auth_required,omitempty"`
	PaymentRequired     bool  `json:"payment_required,omitempty" yaml:"payment_required,omitempty"`
	CreatedAtLowerLimit int64 `json:"created_at_lower_limit,omitempty" yaml:"created_at_lower_limit,omitempty"`
	CreatedAtUpperLimit int64 `json:"created_at_upper_limit,omitempty" yaml:"created_at_upper_limit,omitempty"`
}

type NIP11Retention struct {
	Kinds []*Nip11Kind `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	Time  *int         `json:"time,omitempty" yaml:"time,omitempty"`
	Count *int         `json:"count,omitempty" yaml:"count,omitempty"`
}

type Nip11Kind struct {
	From, To int
}

func (k Nip11Kind) MarshalJSON() ([]byte, error) {
	if k.From == k.To {
		ret, err := json.Marshal(k.From)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Nip11Kind: %w", err)
		}
		return ret, nil
	}

	ret, err := json.Marshal([]int{k.From, k.To})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Nip11Kind: %w", err)
	}
	return ret, nil
}

func (k *Nip11Kind) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
	}

	var ret Nip11Kind

	switch v := v.(type) {
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
		}
		ret.From = int(i)
		ret.To = int(i)
	case []any:
		if len(v) != 2 {
			return fmt.Errorf("failed to unmarshal Nip11Kind: expected 2 elements, got %d", len(v))
		}

		i, ok := v[0].(json.Number)
		if !ok {
			return fmt.Errorf("failed to unmarshal Nip11Kind: expected number, got %T", v[0])
		}
		from, err := i.Int64()
		if err != nil {
			return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
		}

		i, ok = v[1].(json.Number)
		if !ok {
			return fmt.Errorf("failed to unmarshal Nip11Kind: expected number, got %T", v[1])
		}
		to, err := i.Int64()
		if err != nil {
			return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
		}

		ret.From = int(from)
		ret.To = int(to)
	}

	*k = ret

	return nil
}

func (k Nip11Kind) MarshalYAML() (interface{}, error) {
	if k.From == k.To {
		return k.From, nil
	}
	return []int{k.From, k.To}, nil
}

func (k *Nip11Kind) UnmarshalYAML(node *yaml.Node) error {
	var ret Nip11Kind

	switch node.Kind {
	case yaml.ScalarNode:
		var i int
		if err := node.Decode(&i); err != nil {
			return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
		}
		ret.From = i
		ret.To = i
	case yaml.SequenceNode:
		var arr []int
		if err := node.Decode(&arr); err != nil {
			return fmt.Errorf("failed to unmarshal Nip11Kind: %w", err)
		}
		if len(arr) != 2 {
			return fmt.Errorf("failed to unmarshal Nip11Kind: expected 2 elements, got %d", len(arr))
		}
		ret.From = arr[0]
		ret.To = arr[1]
	default:
		return fmt.Errorf("failed to unmarshal Nip11Kind: unexpected node kind %v", node.Kind)
	}

	*k = ret
	return nil
}

type NIP11Fees struct {
	Admission    []*Nip11Fee `json:"admission,omitempty" yaml:"admission,omitempty"`
	Subscription []*Nip11Fee `json:"subscription,omitempty" yaml:"subscription,omitempty"`
	Publication  []*Nip11Fee `json:"publication,omitempty" yaml:"publication,omitempty"`
}

type Nip11Fee struct {
	Kinds  []*Nip11Kind `json:"kinds,omitempty" yaml:"kinds,omitempty"`
	Amount int          `json:"amount" yaml:"amount"`
	Unit   string       `json:"unit,omitempty" yaml:"unit,omitempty"`
	Period *int         `json:"period,omitempty" yaml:"period,omitempty"`
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
