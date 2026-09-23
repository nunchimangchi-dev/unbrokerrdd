package agent

import "testing"

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
