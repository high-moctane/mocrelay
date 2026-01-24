//go:build goexperiment.jsonv2

package mocrelay

// RelayInfo represents NIP-11 Relay Information Document.
// All fields are optional (omitempty).
type RelayInfo struct {
	// Basic information
	Name        string `json:"name,omitzero"`
	Description string `json:"description,omitzero"`
	Banner      string `json:"banner,omitzero"`
	Icon        string `json:"icon,omitzero"`
	Pubkey      string `json:"pubkey,omitzero"` // 32-byte hex, admin contact
	Self        string `json:"self,omitzero"`   // 32-byte hex, relay's own pubkey
	Contact     string `json:"contact,omitzero"`

	SupportedNIPs []int  `json:"supported_nips,omitzero"`
	Software      string `json:"software,omitzero"`
	Version       string `json:"version,omitzero"`

	PrivacyPolicy  string `json:"privacy_policy,omitzero"`
	TermsOfService string `json:"terms_of_service,omitzero"`

	// Server limitations
	Limitation *RelayLimitation `json:"limitation,omitzero"`

	// Event retention policies
	// TODO: kinds field has union type: int | [int, int]
	// For now, use a simplified representation
	Retention []*RelayRetention `json:"retention,omitzero"`

	// Content limitations
	RelayCountries []string `json:"relay_countries,omitzero"` // ISO 3166-1 alpha-2

	// Community preferences
	LanguageTags  []string `json:"language_tags,omitzero"` // IETF language tags
	Tags          []string `json:"tags,omitzero"`          // e.g., "sfw-only", "bitcoin-only"
	PostingPolicy string   `json:"posting_policy,omitzero"`

	// Pay-to-Relay
	PaymentsURL string     `json:"payments_url,omitzero"`
	Fees        *RelayFees `json:"fees,omitzero"`
}

// RelayLimitation represents NIP-11 limitation object.
type RelayLimitation struct {
	MaxMessageLength int64 `json:"max_message_length,omitzero"`
	MaxSubscriptions int   `json:"max_subscriptions,omitzero"`
	MaxLimit         int   `json:"max_limit,omitzero"`
	MaxSubidLength   int   `json:"max_subid_length,omitzero"`
	MaxEventTags     int   `json:"max_event_tags,omitzero"`
	MaxContentLength int   `json:"max_content_length,omitzero"`

	MinPowDifficulty int  `json:"min_pow_difficulty,omitzero"`
	AuthRequired     bool `json:"auth_required,omitzero"`
	PaymentRequired  bool `json:"payment_required,omitzero"`
	RestrictedWrites bool `json:"restricted_writes,omitzero"`

	// created_at limits in seconds
	// Lower: events older than (now - lower_limit) are rejected
	// Upper: events newer than (now + upper_limit) are rejected
	CreatedAtLowerLimit int64 `json:"created_at_lower_limit,omitzero"`
	CreatedAtUpperLimit int64 `json:"created_at_upper_limit,omitzero"`

	DefaultLimit int `json:"default_limit,omitzero"`
}

// RelayRetention represents NIP-11 retention policy.
// TODO: The "kinds" field in NIP-11 can contain:
//   - Single integers: 0, 1, 4
//   - Ranges as tuples: [5, 7], [40, 49], [40000, 49999]
//
// Example from NIP-11:
//
//	{"kinds": [0, 1, [5, 7], [40, 49]], "time": 3600}
//
// For now, we use a simplified representation with separate fields.
// A full implementation would need custom JSON marshaling.
type RelayRetention struct {
	// Simplified: list of single kinds (not ranges)
	// TODO: Support kind ranges like [30000, 39999]
	Kinds []int64 `json:"kinds,omitzero"`

	// Time in seconds. nil means infinity.
	// 0 means the event will not be stored at all.
	Time *int64 `json:"time,omitzero"`

	// Maximum number of events to keep
	Count *int64 `json:"count,omitzero"`
}

// RelayFees represents NIP-11 fee schedules.
type RelayFees struct {
	Admission    []*RelayFee             `json:"admission,omitzero"`
	Subscription []*RelaySubscriptionFee `json:"subscription,omitzero"`
	Publication  []*RelayPublicationFee  `json:"publication,omitzero"`
}

// RelayFee represents a basic fee amount.
type RelayFee struct {
	Amount int64  `json:"amount"`
	Unit   string `json:"unit"` // e.g., "msats", "sats"
}

// RelaySubscriptionFee represents a subscription fee with period.
type RelaySubscriptionFee struct {
	Amount int64  `json:"amount"`
	Unit   string `json:"unit"`
	Period int64  `json:"period"` // in seconds
}

// RelayPublicationFee represents a publication fee for specific kinds.
type RelayPublicationFee struct {
	Kinds  []int64 `json:"kinds,omitzero"`
	Amount int64   `json:"amount"`
	Unit   string  `json:"unit"`
}
