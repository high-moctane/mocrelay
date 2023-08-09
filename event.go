package main

import (
	"fmt"
	"time"

	jsoniter "github.com/json-iterator/go"
)

type KindType int

const (
	KindMetadata                = 0
	KindShortTextNote           = 1
	KindRecommendRelay          = 2
	KindContacts                = 3
	KindEncryptedDirectMessages = 4
	KindEventDeletion           = 5
	KindRepost                  = 6
	KindReaction                = 7
	KindBadgeAward              = 8
	KindGenericRepost           = 16
	KindChannelCreation         = 40
	KindChannelMetadata         = 41
	KindChannelMessage          = 42
	KindChannelHideMessage      = 43
	KindChannelMuteUser         = 44
	KindFileMetadata            = 1063
	KindLiveChatMessage         = 1311
	KindReporting               = 1984
	KindLabel                   = 1985
	KindZapRequest              = 9734
	KindZap                     = 9735
	KindMuteList                = 10000
	KindPinList                 = 10001
	KindRelayListMetadata       = 10002
	KindWalletInfo              = 13194
	KindClientAuthentication    = 22242
	KindWalletRequest           = 23194
	KindWalletResponse          = 23195
	KindNostrConnect            = 24133
	KindHTTPAuth                = 27235
	KindCategorizedPeopleList   = 30000
	KindCategorizedBookmarkList = 30001
	KindProfileBadges           = 30008
	KindBadgeDefinition         = 30009
	KindCreateorupdateastall    = 30017
	KindCreateorupdateaproduct  = 30018
	KindLongformContent         = 30023
	KindDraftLongformContent    = 30024
	KindApplicationSpecificData = 30078
	KindLiveEvent               = 30311
	KindClassifiedListing       = 30402
	KindDraftClassifiedListing  = 30403
	KindDateBasedCalendarEvent  = 31922
	KindTimeBasedCalendarEvent  = 31923
	KindCalendar                = 31924
	KindCalendarEventRSVP       = 31925
	KindHandlerRecommendation   = 31989
	KindHandlerInformation      = 31990
)

type EventType int

const (
	EventUnknown = iota
	EventBasic
	EventRegular
	EventReplaceable
	EventEphemeral
	EventParameterizedReplaceable
)

func NewEvent(json *EventJSON, receivedAt time.Time) *Event {
	return &Event{
		EventJSON:  json,
		ReceivedAt: receivedAt,
	}
}

type Event struct {
	*EventJSON
	ReceivedAt time.Time
}

func (e *Event) ValidCreatedAt() bool {
	sub := time.Until(e.CreatedAtToTime())
	// TODO(high-moctane) no magic number
	return -10*time.Minute <= sub && sub <= 5*time.Minute
}

func (e *Event) MarshalJSON() ([]byte, error) {
	ji := jsoniter.ConfigCompatibleWithStandardLibrary
	return ji.Marshal(e.EventJSON)
}

func (e *Event) EventType() EventType {
	if e == nil {
		return EventUnknown
	}

	kind := e.Kind

	if 0 <= kind && kind <= 999 {
		return EventBasic
	} else if 1000 <= kind && kind <= 9999 {
		return EventRegular
	} else if 10000 <= kind && kind <= 19999 {
		return EventReplaceable
	} else if 20000 <= kind && kind <= 29999 {
		return EventEphemeral
	} else if 30000 <= kind && kind <= 39999 {
		return EventParameterizedReplaceable
	}

	return EventUnknown
}

// TODO(high-moctane) GetAllTagByName?
func (e *Event) GetTagByName(tag string) []string {
	for _, arr := range e.Tags {
		if len(arr) < 2 {
			// TODO(high-moctane) validator is needed
			continue
		}

		if arr[0] == tag {
			return arr
		}
	}

	return nil
}

func (e *Event) DTagValue() *string {
	if e == nil || e.EventType() != EventParameterizedReplaceable {
		return nil
	}

	for _, tag := range e.Tags {
		if len(tag) == 0 {
			continue
		}
		if tag[0] != "d" {
			continue
		}
		if len(tag) == 1 {
			return GetRef("")
		}
		return &tag[1]
	}

	return GetRef("")
}

func (e *Event) Naddr() *string {
	dval := e.DTagValue()
	if dval == nil {
		return nil
	}
	return GetRef(fmt.Sprintf("%d:%s:%s", e.Kind, e.Pubkey, *dval))
}
