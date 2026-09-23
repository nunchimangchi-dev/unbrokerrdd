package agent

import (
	"strings"
	"testing"
)

func TestReachability_Verdict(t *testing.T) {
	cases := []struct {
		name     string
		r        Reachability
		wantDead bool
	}{
		{
			// Two independent failures. This is the only combination that
			// earns a dead_site classification.
			name:     "no dns and no load",
			r:        Reachability{Resolves: false, DNSDefinitive: true, Loaded: false, DNSErr: "no such host"},
			wantDead: true,
		},
		{
			// The case that makes the two-signal rule worth having: a local
			// resolver failure must never on its own condemn a live site.
			name:     "dns failed but browser loaded it",
			r:        Reachability{Resolves: false, DNSDefinitive: true, Loaded: true, BodyChars: 900, Title: "Real Site"},
			wantDead: false,
		},
		{
			name:     "resolves and serves a for-sale page",
			r:        Reachability{Resolves: true, DNSDefinitive: true, Loaded: true, Parked: true, BodyChars: 300, Title: "Buy this domain"},
			wantDead: true,
		},
		{
			// Equally consistent with a slow client-rendered app, so it asks
			// for a human look rather than claiming anything.
			name:     "loads but renders nothing",
			r:        Reachability{Resolves: true, DNSDefinitive: true, Loaded: true, BodyChars: 5},
			wantDead: false,
		},
		{
			name:     "healthy site",
			r:        Reachability{Resolves: true, DNSDefinitive: true, Loaded: true, BodyChars: 5000, Title: "People Search"},
			wantDead: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dead, reason := c.r.Verdict()
			if dead != c.wantDead {
				t.Errorf("Verdict() dead = %v, want %v (reason %q)", dead, c.wantDead, reason)
			}
			if dead && reason == "" {
				t.Error("a dead verdict must say why")
			}
		})
	}
}

// The case that produced ten false dead_site candidates in one sweep: the DNS
// check and the browser check both go through the same resolver, so a
// saturated resolver fails both and reads as corroboration. A non-answer is
// not evidence of absence, however many times it is repeated.
func TestReachability_ResolverTimeoutIsNotDeath(t *testing.T) {
	r := Reachability{Resolves: false, DNSDefinitive: false, Loaded: false,
		DNSErr: "lookup businesssearch.sos.ca.gov: i/o timeout"}
	dead, reason := r.Verdict()
	if dead {
		t.Fatalf("a resolver timeout must never be a dead verdict (reason %q)", reason)
	}
	if reason == "" {
		t.Error("an inconclusive check must say it was inconclusive")
	}
}

func TestReachability_ParkedDetection(t *testing.T) {
	r := Reachability{Resolves: true, DNSDefinitive: true, Loaded: true, BodyChars: 200,
		Title: "domain for sale", Parked: true}
	dead, reason := r.Verdict()
	if !dead {
		t.Fatalf("parked domain should be dead, reason %q", reason)
	}
}

func TestClassifyWhoisEmail_Tiers(t *testing.T) {
	cases := []struct {
		name     string
		addr     string
		context  string
		domain   string
		wantTier Tier
		usable   bool
	}{
		{
			name: "mailbox at the site's own domain",
			addr: "privacy@example.com", context: "Registrant Email: privacy@example.com",
			domain: "example.com", wantTier: TierDirect, usable: true,
		},
		{
			// The distinction that turned "ten dead ends" into "three weak
			// options": a proxy forwarder relays to the registrant, a
			// registrar abuse desk does not.
			name: "privacy proxy forwards to the registrant",
			addr: "abc123.protect@withheldforprivacy.com", context: "Registrant Email: abc123.protect@withheldforprivacy.com",
			domain: "example.com", wantTier: TierForwarded, usable: true,
		},
		{
			name: "registrar abuse desk is not the operator",
			addr: "abuse@namecheap.com", context: "Registrar Abuse Contact Email: abuse@namecheap.com",
			domain: "example.com", wantTier: TierRegistrar, usable: false,
		},
		{
			name: "unrelated third party",
			addr: "someone@unrelated.example", context: "Tech Email: someone@unrelated.example",
			domain: "example.com", wantTier: TierUnknown, usable: false,
		},
		{
			name: "www prefix still counts as the same site",
			addr: "privacy@example.com", context: "Registrant Email: privacy@example.com",
			domain: "www.example.com", wantTier: TierDirect, usable: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyWhoisEmail(c.addr, c.context, c.domain)
			if got.Tier != c.wantTier {
				t.Errorf("tier = %q, want %q (%s)", got.Tier, c.wantTier, got.Why)
			}
			if got.Usable != c.usable {
				t.Errorf("usable = %v, want %v (%s)", got.Usable, c.usable, got.Why)
			}
			if got.Why == "" {
				t.Error("every classification must say why")
			}
		})
	}
}

func TestWhoisResult_BestPrefersDirectOverForwarded(t *testing.T) {
	r := &WhoisResult{Contacts: []WhoisContact{
		{Email: "p@proxy.example", Tier: TierForwarded, Usable: true},
		{Email: "privacy@site.example", Tier: TierDirect, Usable: true},
	}}
	best := r.Best()
	if best == nil || best.Tier != TierDirect {
		t.Fatalf("Best() should prefer a direct mailbox, got %+v", best)
	}
}

func TestWhoisResult_BestIgnoresRegistrar(t *testing.T) {
	r := &WhoisResult{Contacts: []WhoisContact{
		{Email: "abuse@registrar.example", Tier: TierRegistrar, Usable: false},
	}}
	if best := r.Best(); best != nil {
		t.Errorf("Best() returned a registrar desk: %+v", best)
	}
}

func TestParkedSignals_CatchCommonPhrasings(t *testing.T) {
	// Each of these is a real parked-page rendering. The list previously held
	// only the longer phrasings and reported a for-sale domain as alive.
	for _, page := range []string{
		"Newcon.com for sale | Spaceship.com — Domain for sale",
		"This domain is for sale, make an offer",
		"Buy this domain",
		"example.io for sale",
		"opendatausa.com is registered at Namecheap",
	} {
		low := strings.ToLower(page)
		matched := false
		for _, sig := range parkedSignals {
			if strings.Contains(low, sig) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("no parked signal matched %q", page)
		}
	}
}
