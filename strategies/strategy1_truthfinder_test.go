package strategies_test

import (
	"context"
	"testing"
	"time"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/dashboard"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
	"github.com/nunchimangchi-dev/unbrokerrdd/strategies"
)

// ── Allowlist tests ───────────────────────────────────────────────────────────

func TestAllowlist_TruthFinderPermitted(t *testing.T) {
	urls := []string{
		"https://suppression.peopleconnect.us/?brand=TruthFinder",
		"https://suppression.peopleconnect.us/login",
		"http://suppression.peopleconnect.us/login", // http also allowed
	}
	for _, u := range urls {
		if err := agent.ValidateURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}
}

func TestAllowlist_UnknownDomainBlocked(t *testing.T) {
	blocked := []string{
		"https://malicious.com/steal",
		"https://evil.truthfinder.com.attacker.io/form", // subdomain spoofing
		"https://google.com",
		"https://anthropic.com",
		"ftp://truthfinder.com/privacy-center",           // wrong scheme
		"javascript:alert(1)",                            // script injection
		"",                                               // empty
	}
	for _, u := range blocked {
		if err := agent.ValidateURL(u); err == nil {
			t.Errorf("expected %q to be blocked, but it was allowed", u)
		}
	}
}

func TestAllowlist_AllBrokerURLsPass(t *testing.T) {
	// Every domain referenced in AllowedDomains must round-trip through ValidateURL.
	for domain := range agent.AllowedDomains {
		u := "https://" + domain + "/opt-out"
		if err := agent.ValidateURL(u); err != nil {
			t.Errorf("allowlist domain %q failed its own check: %v", domain, err)
		}
	}
}

// ── Registry integrity ───────────────────────────────────────────────────────

// TestRegistry_NoDuplicateIDs - broker.ID is the primary key and the switch
// key every Strategy 2 handler routes on. A duplicate would silently shadow
// a handler and Seed would quietly ignore the second row.
func TestRegistry_NoDuplicateIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range dashboard.AllBrokers() {
		if seen[b.ID] {
			t.Errorf("duplicate broker ID in registry: %q", b.ID)
		}
		seen[b.ID] = true
	}
}

// TestRegistry_EveryBrokerIsAllowlisted closes a real gap: the allowlist test
// above only proves allowlisted domains validate, not that every broker the
// registry declares is actually permitted to be navigated to. Adding a broker
// and forgetting its allowlist entry would otherwise surface as a confusing
// runtime "domain not in the broker allowlist" failure during a live run,
// rather than here.
func TestRegistry_EveryBrokerIsAllowlisted(t *testing.T) {
	for _, b := range dashboard.AllBrokers() {
		if err := agent.ValidateURL("https://" + b.URL + "/"); err != nil {
			t.Errorf("registry broker %q (%s) is not allowlisted: %v", b.ID, b.URL, err)
		}
	}
}

// ── Strategy 1 dry-run ────────────────────────────────────────────────────────

func TestStrategy1_DryRunNeverSubmits(t *testing.T) {
	s := strategies.NewStrategy1()
	broker := db.Broker{
		ID:       "backgroundcheckme",
		Name:     "Backgroundcheckme.org",
		Strategy: 1,
		Status:   db.StatusPending,
	}
	cfg := &config.Config{} // empty — dry-run must not require config

	result, err := s.Run(context.Background(), broker, cfg, true /* dryRun */)
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Error("result.DryRun should be true")
	}
	// Dry-run must leave status as pending — it doesn't count as an attempt
	if result.Status != db.StatusPending {
		t.Errorf("dry-run should return StatusPending, got %q", result.Status)
	}
	if result.Notes == "" {
		t.Error("dry-run should include notes describing what would happen")
	}
	t.Logf("dry-run notes: %s", result.Notes)
}

func TestStrategy1_LiveRunBlockedWithoutConfig(t *testing.T) {
	s := strategies.NewStrategy1()
	broker := db.Broker{
		ID:       "backgroundcheckme",
		Strategy: 1,
		Status:   db.StatusPending,
	}
	cfg := &config.Config{} // empty — live run must be blocked

	result, err := s.Run(context.Background(), broker, cfg, false /* live */)
	if err == nil {
		t.Fatal("live run without config should return an error")
	}
	// Should not silently succeed
	if result.Status == db.StatusSuccess {
		t.Error("live run without config must not return success")
	}
	t.Logf("correctly blocked: %v", err)
}

// ── Idempotency (DB layer) ────────────────────────────────────────────────────

func TestStore_CanDispatch_PendingOnly(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	// Seed one broker
	err := store.Seed([]db.Broker{
		{ID: "test-broker", Name: "Test", Strategy: 1, URL: "https://truthfinder.com"},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Should be dispatchable when pending
	ok, err := store.CanDispatch("test-broker")
	if err != nil {
		t.Fatalf("CanDispatch: %v", err)
	}
	if !ok {
		t.Error("pending broker should be dispatchable")
	}

	// After marking in-progress, should NOT be dispatchable
	if err := store.SetInProgress("test-broker"); err != nil {
		t.Fatalf("SetInProgress: %v", err)
	}
	ok, err = store.CanDispatch("test-broker")
	if err != nil {
		t.Fatalf("CanDispatch after in-progress: %v", err)
	}
	if ok {
		t.Error("in-progress broker must NOT be dispatchable (idempotency)")
	}
}

func TestStore_Seed_IsIdempotent(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	brokers := []db.Broker{
		{ID: "b1", Name: "Broker 1", Strategy: 1, URL: "https://truthfinder.com"},
	}

	// Seed twice — second call must not error or reset state
	if err := store.Seed(brokers); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// Manually advance the broker
	if err := store.SetInProgress("b1"); err != nil {
		t.Fatalf("SetInProgress: %v", err)
	}
	if err := store.Settle("b1", db.StatusSuccess, false, "ok", "test", ""); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	// Second seed must NOT reset it back to pending
	if err := store.Seed(brokers); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	all, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 || all[0].Status != db.StatusSuccess {
		t.Errorf("second seed reset broker to pending — idempotency broken. Status: %q", all[0].Status)
	}
}

func TestStore_Settle_LogsAttempt(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	_ = store.Settle("b1", db.StatusSuccess, true, "confirmed", "dry-run test", "https://confirm.example.com")

	all, _ := store.GetAll()
	if all[0].AttemptCount != 1 {
		t.Errorf("attempt_count should be 1, got %d", all[0].AttemptCount)
	}
	if all[0].ConfirmationURL != "https://confirm.example.com" {
		t.Errorf("confirmation_url not stored correctly: %q", all[0].ConfirmationURL)
	}
}

// TestStore_LastLiveAttemptAt_RealAttempt pins a real bug (2026-09-22):
// LastLiveAttemptAt scanned MAX(created_at) straight into *time.Time, which
// works for a direct column select (see scanBrokers) but not for an
// aggregate result, which loses the type-affinity hint and comes back as
// plain TEXT - Scan failed with "unsupported Scan ... string into *time.Time"
// the first time this ran against a broker with real attempt history.
// TestStore_Settle_LogsAttempt above never caught this because its only
// Settle call uses dryRun=true, and this method's query filters dry_run=0 -
// a broker with zero matching rows returns SQL NULL, which scans into a nil
// *time.Time fine. The bug only ever showed up with a real dry_run=0 row
// present, which is deliberately what this test exercises.
func TestStore_LastLiveAttemptAt_RealAttempt(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	if err := store.Settle("b1", db.StatusFailed, false /* dryRun */, "failed", "live attempt", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	got, err := store.LastLiveAttemptAt("b1")
	if err != nil {
		t.Fatalf("LastLiveAttemptAt returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil timestamp for a broker with a real attempt logged")
	}
	if time.Since(*got) > time.Minute {
		t.Errorf("timestamp too far in the past, parsing likely wrong: %v", got)
	}
}

// TestStore_LastLiveAttemptAt_NoAttempts covers the complementary case -
// a broker with no attempts at all must return (nil, nil), not an error.
func TestStore_LastLiveAttemptAt_NoAttempts(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})

	got, err := store.LastLiveAttemptAt("b1")
	if err != nil {
		t.Fatalf("LastLiveAttemptAt returned error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a broker with no attempts, got %v", got)
	}
}

// TestStore_LastLiveAttemptAt_IgnoresManualRouting pins a real bug: a
// manual-routed outcome (no verified handler, or a missing required input
// like profile_url) returns before any navigation, so it never contacts the
// broker - but it was still logged as a live attempt and gated the 48h
// cooldown. That blocked Spokeo's first genuine run for two days over a
// result that had never touched spokeo.com.
func TestStore_LastLiveAttemptAt_IgnoresManualRouting(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 2, URL: "https://spokeo.com"}})
	_ = store.SetInProgress("b1")
	_ = store.Settle("b1", db.StatusManual, false /* live */, "manual", "no profile_url set", "")

	got, err := store.LastLiveAttemptAt("b1")
	if err != nil {
		t.Fatalf("LastLiveAttemptAt: %v", err)
	}
	if got != nil {
		t.Errorf("a manual-routed attempt never contacts the site and must not gate the cooldown, got %v", got)
	}
}

// ── Completion method (automation coverage accounting) ───────────────────────

// TestStore_Settle_LiveSuccessIsAutonomous covers the one path that may
// claim an autonomous completion: the agent succeeding on a real run.
func TestStore_Settle_LiveSuccessIsAutonomous(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	if err := store.Settle("b1", db.StatusSuccess, false /* live */, "ok", "confirmed", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	all, _ := store.GetAll()
	if all[0].CompletionMethod != db.CompletionAutonomous {
		t.Errorf("live success should record autonomous, got %q", all[0].CompletionMethod)
	}
}

// TestStore_Settle_DryRunNeverClaimsAutonomous guards the obvious way this
// accounting could quietly inflate: a dry run never submits anything, so it
// must never look like the automation completed a removal.
func TestStore_Settle_DryRunNeverClaimsAutonomous(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	_ = store.Settle("b1", db.StatusSuccess, true /* dryRun */, "ok", "dry run", "")

	all, _ := store.GetAll()
	if all[0].CompletionMethod == db.CompletionAutonomous {
		t.Error("a dry run must never be recorded as an autonomous completion")
	}
}

// TestStore_Settle_FailureIsNotACompletion - only successes are completions.
func TestStore_Settle_FailureIsNotACompletion(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	_ = store.Settle("b1", db.StatusFailed, false /* live */, "nope", "failed", "")

	all, _ := store.GetAll()
	if all[0].CompletionMethod != db.CompletionNone {
		t.Errorf("a failed run should record no completion method, got %q", all[0].CompletionMethod)
	}
}

func TestStore_SetCompletionMethod_Human(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	_ = store.Seed([]db.Broker{{ID: "b1", Name: "B1", Strategy: 1, URL: "https://truthfinder.com"}})
	_ = store.SetInProgress("b1")
	_ = store.Settle("b1", db.StatusSuccess, false, "ok", "done by hand", "")
	if err := store.SetCompletionMethod("b1", db.CompletionHuman); err != nil {
		t.Fatalf("SetCompletionMethod: %v", err)
	}

	stats, err := store.CompletionStats()
	if err != nil {
		t.Fatalf("CompletionStats: %v", err)
	}
	if stats[db.CompletionHuman] != 1 {
		t.Errorf("expected 1 human-completed, got %d", stats[db.CompletionHuman])
	}
	if stats[db.CompletionAutonomous] != 0 {
		t.Errorf("expected 0 autonomous after correction, got %d", stats[db.CompletionAutonomous])
	}
}

// ── Registry/database reconciliation ─────────────────────────────────────────

// TestStore_OrphanedBrokerIDs pins the drift that went unnoticed for days:
// Seed is INSERT OR IGNORE, so a broker removed from the registry keeps its
// row (and keeps counting) until something actively looks for it.
func TestStore_OrphanedBrokerIDs(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	seeded := []db.Broker{
		{ID: "kept", Name: "Kept", Strategy: 2, URL: "https://checkpeople.com"},
		{ID: "removed-from-code", Name: "Gone", Strategy: 2, URL: "https://checkpeople.com"},
	}
	_ = store.Seed(seeded)

	// Registry no longer declares the second broker.
	registry := []db.Broker{seeded[0]}

	orphans, err := store.OrphanedBrokerIDs(registry)
	if err != nil {
		t.Fatalf("OrphanedBrokerIDs: %v", err)
	}
	if len(orphans) != 1 || orphans[0] != "removed-from-code" {
		t.Errorf("expected exactly [removed-from-code], got %v", orphans)
	}
}

func TestStore_OrphanedBrokerIDs_CleanRegistry(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	seeded := []db.Broker{{ID: "kept", Name: "Kept", Strategy: 2, URL: "https://checkpeople.com"}}
	_ = store.Seed(seeded)

	orphans, err := store.OrphanedBrokerIDs(seeded)
	if err != nil {
		t.Fatalf("OrphanedBrokerIDs: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans, got %v", orphans)
	}
}

// ── Config redaction ──────────────────────────────────────────────────────────

func TestConfig_RedactedNeverExposesSecrets(t *testing.T) {
	cfg := &config.Config{
		SubjectName:  "John Smith",
		SubjectEmail: "john@example.com",
		AnthropicKey: "sk-ant-realkey123456",
	}
	r := cfg.Redacted()

	if r.AnthropicKey == cfg.AnthropicKey {
		t.Error("Redacted() must not expose the Anthropic API key")
	}
	if r.SubjectEmail == cfg.SubjectEmail {
		t.Error("Redacted() must not expose the full email address")
	}
	t.Logf("redacted config: name=%q email=%q key=%q",
		r.SubjectName, r.SubjectEmail, r.AnthropicKey)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func tempStore(t *testing.T) (*db.Store, func()) {
	t.Helper()
	path := t.TempDir() + "/test.db"
	store, err := db.New(path)
	if err != nil {
		t.Fatalf("create temp store: %v", err)
	}
	return store, func() { store.Close() }
}
