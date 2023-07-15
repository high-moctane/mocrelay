package main

import (
	"encoding/json"
	"testing"
)

func TestNIP11_MarshalUnmarshal(t *testing.T) {
	nip11str := `{"name":"name","description":"description","pubkey":"pubkey","contact":"contact","supported_nips":[1,2,3],"software":"software","version":"version","limitation":{"max_message_length":0,"max_subscriptions":1,"max_filters":2,"max_limit":3,"max_subid_length":4,"min_prefix":5,"max_event_tags":6,"max_content_length":7,"min_pow_difficulty":8,"auth_required":true,"payment_required":false},"retention":[{"kinds":[0,1,[2,10],[18,40],100],"time":123,"count":43},{"kinds":[0,100]}],"relay_countries":["relay_countries"],"language_tags":["language_tags"],"tags":["tags"],"posting_policy":"posting_policy","payments_url":"payments_url","fees":{"admission":[{"amount":10,"unit":"msats","period":493}],"subscription":[]},"icon":"icon"}`

	var nip11 NIP11
	if err := json.Unmarshal([]byte(nip11str), &nip11); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	buf, err := json.Marshal(nip11)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if nip11str != string(buf) {
		t.Fatalf("got %v, want %v", string(buf), nip11str)
	}
}
