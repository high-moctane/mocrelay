package mocrelay

import "strconv"

// knownKinds is the set of Nostr event kinds that get their own Prometheus
// `kind` label value in kind-tagged metrics (EventsReceived, EventsStored).
// Kinds outside this set are collapsed to `kind="other"` so a hostile or
// buggy client cannot explode the series set by sending arbitrary int64
// kind values. The `type` label (see kindLabels) orthogonalises the
// NIP-01 category so operators still see traffic shape.
//
// The list follows the mocrelay metrics policy (CLAUDE.md, Known concerns
// #1): commonly observed kinds get per-kind fidelity; everything else is
// aggregated. Adjust based on operational experience.
var knownKinds = map[int64]struct{}{
	// NIP-01 core
	0: {}, // metadata
	1: {}, // short text note

	// NIP-02
	3: {}, // follows

	// DM family (NIP-04 legacy, NIP-17, NIP-59)
	4:     {}, // legacy direct message
	13:    {}, // seal
	14:    {}, // chat message
	1059:  {}, // gift wrap
	10050: {}, // DM relay list

	// NIP-09 / NIP-18 / NIP-25
	5: {}, // event deletion
	6: {}, // repost
	7: {}, // reaction

	// NIP-22
	1111: {}, // comment

	// NIP-28 public chat
	40: {}, // channel create
	41: {}, // channel metadata
	42: {}, // channel message
	43: {}, // hide message
	44: {}, // mute user

	// NIP-56 reporting
	1984: {},

	// NIP-57 zaps
	9734: {}, // zap request
	9735: {}, // zap receipt

	// NIP-65 relay list
	10002: {},

	// NIP-23 long-form content
	30023: {},
}

// kindLabels returns the (kind, type) Prometheus label values for an event
// kind. The two labels carry redundant information (type is a function of
// kind) but pairing them lets operators query per-kind fidelity and
// per-category shape in the same metric without inflating cardinality —
// `(kind, type)` is a functional relationship, so the observed series set
// is bounded at len(knownKinds) + 5 = 28, not the label cross-product.
//
// The first return is the `kind` label: decimal form of `kind` if in the
// known-kind allowlist, otherwise "other". Cardinality: len(knownKinds) + 1.
//
// The second return is the `type` label, classifying `kind` by NIP-01
// range:
//
//	"regular"     — kind 1, 2, 4..44, 1000..9999
//	"replaceable" — kind 0, 3, 10000..19999
//	"ephemeral"   — kind 20000..29999
//	"addressable" — kind 30000..39999
//	"unknown"     — everything else (e.g. 45..999, ≥ 40000, negative)
//
// Cardinality: 5.
func kindLabels(kind int64) (kindValue, typeValue string) {
	typeValue = kindType(kind)
	if _, ok := knownKinds[kind]; ok {
		return strconv.FormatInt(kind, 10), typeValue
	}
	return "other", typeValue
}

// kindType classifies a kind into one of the NIP-01 categories (regular,
// replaceable, ephemeral, addressable) or "unknown" if the kind falls in
// a gap the spec does not define (e.g. 45..999, values ≥ 40000, negative
// values). This is used as the `type` label on kind-tagged metrics.
func kindType(kind int64) string {
	switch {
	case kind == 0, kind == 3, kind >= 10000 && kind <= 19999:
		return "replaceable"
	case kind >= 20000 && kind <= 29999:
		return "ephemeral"
	case kind >= 30000 && kind <= 39999:
		return "addressable"
	case kind == 1, kind == 2, kind >= 4 && kind <= 44, kind >= 1000 && kind <= 9999:
		return "regular"
	}
	return "unknown"
}
