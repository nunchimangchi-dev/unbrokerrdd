package strategies_test

import (
	"context"
	"testing"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
	"github.com/nunchimangchi-dev/unbrokerrdd/strategies"
)

// ── Allowlist tests ───────────────────────────────────────────────────────────

func TestAllowlist_CheckPeoplePermitted(t *testing.T) {
	urls := []string{
		"https://checkpeople.com/opt-out",
		"https://www.checkpeople.com/opt-out",
	}
	for _, u := range urls {
		if err := agent.ValidateURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}
}

func TestAllowlist_AdvancedBackgroundChecksPermitted(t *testing.T) {
	urls := []string{
		"https://www.advancedbackgroundchecks.com/opt-out",
		"https://advancedbackgroundchecks.com/opt-out",
	}
	for _, u := range urls {
		if err := agent.ValidateURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}
}

// TestStrategy2_AdvancedBackgroundChecksDryRunNeverSubmits mirrors the
// CheckPeople dry-run test for the second Strategy 2 site.
func TestStrategy2_AdvancedBackgroundChecksDryRunNeverSubmits(t *testing.T) {
	s := strategies.NewStrategy2()
	broker := db.Broker{ID: "advancedbackgroundchecks", Name: "AdvancedBackgroundChecks", Strategy: 2, Status: db.StatusPending}
	cfg := &config.Config{}

	result, err := s.Run(context.Background(), broker, cfg, true /* dryRun */)
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Error("result.DryRun should be true")
	}
	if result.Status != db.StatusPending {
		t.Errorf("dry-run should return StatusPending, got %q", result.Status)
	}
}

// ── Strategy 2 dry-run / gating ────────────────────────────────────────────────

func TestStrategy2_DryRunNeverSubmits(t *testing.T) {
	s := strategies.NewStrategy2()
	broker := db.Broker{ID: "checkpeople", Name: "CheckPeople", Strategy: 2, Status: db.StatusPending}
	cfg := &config.Config{} // empty — dry-run must not require config

	result, err := s.Run(context.Background(), broker, cfg, true /* dryRun */)
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Error("result.DryRun should be true")
	}
	if result.Status != db.StatusPending {
		t.Errorf("dry-run should return StatusPending, got %q", result.Status)
	}
	if result.Notes == "" {
		t.Error("dry-run should include notes describing what would happen")
	}
}

func TestStrategy2_LiveRunBlockedWithoutConfig(t *testing.T) {
	s := strategies.NewStrategy2()
	broker := db.Broker{ID: "checkpeople", Strategy: 2, Status: db.StatusPending}
	cfg := &config.Config{} // empty — live run must be blocked

	result, err := s.Run(context.Background(), broker, cfg, false /* live */)
	if err == nil {
		t.Fatal("live run without config should return an error")
	}
	if result.Status == db.StatusSuccess {
		t.Error("live run without config must not return success")
	}
}

// TestStrategy2_UnknownBrokerRoutesToManual verifies that any broker in the
// Strategy 2 bucket without a verified handler is reported as StatusManual
// on a live run — rather than attempted with unverified selectors. This is
// deliberate (see the Strategy2 doc comment), not a gap to silently paper over.
func TestStrategy2_UnknownBrokerRoutesToManual(t *testing.T) {
	s := strategies.NewStrategy2()
	broker := db.Broker{ID: "ohioresidentdirectory", Name: "Ohio Resident Directory", Strategy: 2, Status: db.StatusPending}
	cfg := &config.Config{}

	result, err := s.Run(context.Background(), broker, cfg, false /* live */)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != db.StatusManual {
		t.Errorf("unimplemented broker should route to StatusManual, got %q", result.Status)
	}
	if result.Notes == "" {
		t.Error("manual routing should explain why in Notes")
	}
}

// TestStrategy2_UnknownBrokerDryRunStaysPending verifies dry-run never
// changes persisted state, even for the "no handler yet" fallback path —
// it must stay StatusPending (dispatchable again later), not jump straight
// to StatusManual the way a real run would.
func TestStrategy2_UnknownBrokerDryRunStaysPending(t *testing.T) {
	s := strategies.NewStrategy2()
	broker := db.Broker{ID: "ohioresidentdirectory", Name: "Ohio Resident Directory", Strategy: 2, Status: db.StatusPending}
	cfg := &config.Config{} // empty — dry-run must not require config

	result, err := s.Run(context.Background(), broker, cfg, true /* dryRun */)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != db.StatusPending {
		t.Errorf("dry-run fallback should stay StatusPending, got %q", result.Status)
	}
	if !result.DryRun {
		t.Error("result.DryRun should be true")
	}
}
