package handlers

import (
	"encoding/json"
	"testing"

	"automation-developer-guide/src/models"
	"automation-developer-guide/src/ondc"
)

func TestToUserOrgConfigs_KeepsValuesAsSentButTrimsSpaces(t *testing.T) {
	got := toUserOrgConfigs([]ondc.Config{{
		EnvironmentAccessID: " 5128 ", Environment: "Pre-Prod ", Status: " Subscribed", RequestType: "Update Request",
		SubscriberID: " a.com ", SubscriberURL: " https://a.com/ ", Domain: " ONDC:RET10 ", Version: " v 1.2 ",
		UseCase: " Grocery ", NPType: " BPP ",
	}})
	want := models.UserOrgConfig{
		EnvironmentAccessID: "5128", Environment: "Pre-Prod", Status: "Subscribed", RequestType: "Update Request",
		SubscriberID: "a.com", SubscriberURL: "https://a.com/", Domain: "ONDC:RET10", Version: "v 1.2",
		UseCase: "Grocery", NPType: "BPP",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestToUserOrgConfigs_NilGivesEmptyList(t *testing.T) {
	if got := toUserOrgConfigs(nil); got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want an empty (non-nil) list", got)
	}
}

func TestOrgConfigsForProfile_CamelCaseAndNeverNull(t *testing.T) {
	if got := orgConfigsForProfile(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil configs: got %#v, want []", got)
	}
	got := orgConfigsForProfile([]models.UserOrgConfig{{SubscriberURL: "https://a.com/", NPType: "BAP", Domain: "ONDC:RET10", Version: "1.2.5", UseCase: "GROCERY", EnvironmentAccessID: "9"}})
	if len(got) != 1 {
		t.Fatalf("got %d configs", len(got))
	}
	// Check the JSON the website receives: camelCase keys, and exactly 10 of them
	// (so adding a field to the model without exposing it here is caught).
	raw, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m) != 10 {
		t.Errorf("got %d keys, want 10: %v", len(m), m)
	}
	for key, want := range map[string]string{"subscriberUrl": "https://a.com/", "npType": "BAP", "domain": "ONDC:RET10", "version": "1.2.5", "useCase": "GROCERY", "environmentAccessId": "9"} {
		if m[key] != want {
			t.Errorf("%s = %v, want %q", key, m[key], want)
		}
	}
}
