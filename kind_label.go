package mocrelay

import "strconv"

// knownKinds is the set of Nostr event kinds that get their own Prometheus
// label value in kind-tagged metrics (EventsReceived, EventsStored). Kinds
// outside this set are bucketed by NIP-01 category (see kindLabel) so a
// hostile or buggy client cannot explode the series set by sending
// arbitrary int64 kind values.
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

// kindLabel returns a Prometheus label value for an event kind with bounded
// cardinality. Known kinds return their decimal form (e.g. 1 -> "1").
// Unknown kinds are bucketed by NIP-01 range:
//
//	10000..19999 (replaceable range)   -> "replaceable_other"
//	20000..29999 (ephemeral range)     -> "ephemeral"
//	30000..39999 (addressable range)   -> "addressable_other"
//	1..9999      (regular range)       -> "regular_other"
//	everything else                    -> "other"
//
// Total label cardinality is bounded at len(knownKinds) + 5 regardless of
// client behaviour.
func kindLabel(kind int64) string {
	if _, ok := knownKinds[kind]; ok {
		return strconv.FormatInt(kind, 10)
	}
	switch {
	case kind >= 10000 && kind <= 19999:
		return "replaceable_other"
	case kind >= 20000 && kind <= 29999:
		return "ephemeral"
	case kind >= 30000 && kind <= 39999:
		return "addressable_other"
	case kind >= 1 && kind <= 9999:
		return "regular_other"
	}
	return "other"
}
