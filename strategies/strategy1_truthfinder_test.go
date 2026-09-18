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
