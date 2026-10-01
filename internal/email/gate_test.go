package email

import (
	"strings"
	"testing"
)

func TestIsProxyContact(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{"whois-proxy:whois.namecheap.com", true},
		{"whois-proxy:", true},
		{"https://example.com/privacy", false},
		{"", false},
		// Only the recorded tag counts, not a URL that happens to mention it.
		{"https://example.com/whois-proxy:", false},
	} {
		if got := IsProxyContact(tc.source); got != tc.want {
			t.Errorf("IsProxyContact(%q) = %v, want %v", tc.source, got, tc.want)
		}
	}
}

func TestPresenceGate(t *testing.T) {
	for _, tc := range []struct {
		presence string
		want     Verdict
		reason   string
	}{
		{"present", Allow, ""},
		{"absent", Refuse, "no record"},
		{"not_applicable", Refuse, "cannot apply"},
		{"undetermined", Warn, "could not be trusted"},
		{"", Warn, "never been checked"},
		// A presence value this code has never heard of must not open the gate.
		{"someday_new", Warn, "unrecognised"},
	} {
		got, why := PresenceGate(tc.presence)
		if got != tc.want {
			t.Errorf("PresenceGate(%q) = %v, want %v", tc.presence, got, tc.want)
		}
		if tc.reason != "" && !strings.Contains(why, tc.reason) {
			t.Errorf("PresenceGate(%q) reason = %q, want it to mention %q", tc.presence, why, tc.reason)
		}
		if tc.want == Allow && why != "" {
			t.Errorf("PresenceGate(%q) gave a reason %q for an allowed request", tc.presence, why)
		}
	}
}
