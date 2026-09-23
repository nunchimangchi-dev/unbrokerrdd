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

// ── Probe classification ─────────────────────────────────────────────────────

// TestProbe_SuggestedBlocker covers the mapping from what a probe observes to
// the blocker_type it implies - the three shapes that were each misclassified
// by hand at least once: a challenge interstitial, a CAPTCHA, and an HTTP
// block that a normal browser never sees.
func TestProbe_SuggestedBlocker(t *testing.T) {
	cases := []struct {
		name string
		res  agent.ProbeResult
		want string
	}{
		{"cloudflare challenge", agent.ProbeResult{Title: "Just a moment...", Challenge: true}, "bot_defended"},
		{"captcha present", agent.ProbeResult{Title: "Opt out", Captcha: true}, "bot_defended"},
		{"http 403 to automation", agent.ProbeResult{Title: "403 Forbidden"}, "bot_defended"},
		// A navigation error means we never got a response, so it is not
		// evidence about the site at all. This case previously asserted
		// "bot_defended" and that assertion was wrong: a local Chrome launch
		// failure on 2026-09-23 was duly reported as a site defence, which is
		// the precise misclassification probe exists to prevent.
		{"navigation error says nothing about the site", agent.ProbeResult{NavErr: "context deadline exceeded"}, ""},
		{"navigation error outranks a stale title", agent.ProbeResult{NavErr: "chrome failed to start", Title: "403 Forbidden"}, ""},
		{"workable form", agent.ProbeResult{
			Title:  "Opt-Out",
			Inputs: []agent.ProbeInput{{Name: "email", Type: "email", Visible: true}},
		}, ""},
	}
	for _, tc := range cases {
		if got := tc.res.SuggestedBlocker(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
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

// ── Presence ──────────────────────────────────────────────────────────────────

func seedOne(t *testing.T, store *db.Store, id string, status db.Status) {
	t.Helper()
	if err := store.Seed([]db.Broker{{ID: id, Name: id, Strategy: 4, URL: id + ".com", Status: status}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func getBroker(t *testing.T, store *db.Store, id string) db.Broker {
	t.Helper()
	all, err := store.GetAll()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	for _, b := range all {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("broker %q not found", id)
	return db.Broker{}
}

func TestStore_SetPresence_AbsentSkipsButIsNotASuccess(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "absentsite", db.StatusPending)

	if err := store.SetPresence("absentsite", db.PresenceAbsent, "no results"); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	b := getBroker(t, store, "absentsite")
	if b.Presence != db.PresenceAbsent {
		t.Errorf("presence = %q, want absent", b.Presence)
	}
	if b.Status != db.StatusSkipped {
		t.Errorf("status = %q, want skipped", b.Status)
	}
	// The whole point of keeping presence on its own axis: establishing that
	// there was never a record must never look like a removal.
	if b.CompletionMethod != db.CompletionNone {
		t.Errorf("completion_method = %q, want empty — absence is not a completion", b.CompletionMethod)
	}
	if b.PresenceCheckedAt == nil {
		t.Error("presence_checked_at was not stamped")
	}
}

func TestStore_SetPresence_NeverOverwritesRealWork(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "donesite", db.StatusPending)
	if err := store.Settle("donesite", db.StatusSuccess, false, "ok", "", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	if err := store.SetPresence("donesite", db.PresenceAbsent, "no results"); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	b := getBroker(t, store, "donesite")
	if b.Status != db.StatusSuccess {
		t.Errorf("status = %q, want success — a presence check must not undo a completed removal", b.Status)
	}
}

func TestStore_SetPresence_DoesNotLogAnAttempt(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "lookedatsite", db.StatusPending)

	if err := store.SetPresence("lookedatsite", db.PresencePresent, "found"); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	// Looking at a site is not attempting it. If presence checks logged
	// attempts they would trip the 48h submission cooldown, which is the same
	// bug manual routing caused before it was excluded from LastLiveAttemptAt.
	at, err := store.LastLiveAttemptAt("lookedatsite")
	if err != nil {
		t.Fatalf("last live attempt: %v", err)
	}
	if at != nil {
		t.Errorf("presence check logged an attempt at %v; it must not", at)
	}
	if b := getBroker(t, store, "lookedatsite"); b.AttemptCount != 0 {
		t.Errorf("attempt_count = %d, want 0", b.AttemptCount)
	}
}

func TestStore_PresenceStats_CountsUncheckedRegistry(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "a", db.StatusPending)
	seedOne(t, store, "b", db.StatusPending)
	if err := store.SetPresence("a", db.PresencePresent, "found"); err != nil {
		t.Fatalf("set presence: %v", err)
	}

	stats, err := store.PresenceStats()
	if err != nil {
		t.Fatalf("presence stats: %v", err)
	}
	if stats[db.PresencePresent] != 1 {
		t.Errorf("present = %d, want 1", stats[db.PresencePresent])
	}
	if stats[db.PresenceUnknown] != 1 {
		t.Errorf("unknown = %d, want 1 — unchecked targets must stay visible in the denominator", stats[db.PresenceUnknown])
	}
}

func TestStore_Seed_CorrectsAStaleURLWithoutTouchingState(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	if err := store.Seed([]db.Broker{{ID: "movedsite", Name: "Moved", Strategy: 4, URL: "old.example.gov"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.SetBlocker("movedsite", db.BlockerBotDefended, ""); err != nil {
		t.Fatalf("set blocker: %v", err)
	}
	if err := store.Settle("movedsite", db.StatusManual, false, "routed", "hand note", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	// The registry corrects the URL; re-seeding must carry that through.
	if err := store.Seed([]db.Broker{{ID: "movedsite", Name: "Moved Service", Strategy: 4, URL: "new.example.gov"}}); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	b := getBroker(t, store, "movedsite")
	if b.URL != "new.example.gov" {
		t.Errorf("url = %q, want the corrected one — a registry that cannot correct itself is not a source of truth", b.URL)
	}
	if b.Name != "Moved Service" {
		t.Errorf("name = %q, want the corrected one", b.Name)
	}
	// Everything that is live state must survive untouched.
	if b.Status != db.StatusManual {
		t.Errorf("status = %q, want manual — re-seeding must never reset live state", b.Status)
	}
	if b.BlockerType != db.BlockerBotDefended {
		t.Errorf("blocker_type = %q, want bot_defended", b.BlockerType)
	}
	if b.Notes != "hand note" {
		t.Errorf("notes = %q, want the note preserved", b.Notes)
	}
	if b.AttemptCount != 1 {
		t.Errorf("attempt_count = %d, want 1", b.AttemptCount)
	}
}

// ── Attempt history ───────────────────────────────────────────────────────────

func TestStore_Settle_RecordsWhyItFailed(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "failsite", db.StatusPending)

	detail := "agent error: context deadline exceeded waiting for input[name=\"login-email\"]"
	if err := store.Settle("failsite", db.StatusFailed, false, detail, detail, ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	hist, err := store.AttemptHistory("failsite")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("got %d attempts, want 1", len(hist))
	}
	// The bug this guards: eighteen attempts were logged with result "failed"
	// and an empty error column, so "why did this fail" was unanswerable and a
	// later investigation into whether a run had submitted a mistyped address
	// could not be resolved from the database at all.
	if hist[0].Detail != detail {
		t.Errorf("detail = %q, want the real reason", hist[0].Detail)
	}
	if hist[0].Error != detail {
		t.Errorf("error = %q, want the failure reason recorded", hist[0].Error)
	}
	if hist[0].Status != db.StatusFailed {
		t.Errorf("status = %q, want failed", hist[0].Status)
	}
	if hist[0].DryRun {
		t.Error("attempt was live, recorded as dry run")
	}
}

func TestStore_Settle_SuccessLeavesErrorEmpty(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "oksite", db.StatusPending)

	if err := store.Settle("oksite", db.StatusSuccess, false, "confirmation page shown", "", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}
	hist, err := store.AttemptHistory("oksite")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if hist[0].Error != "" {
		t.Errorf("error = %q on a success; the column is for failures", hist[0].Error)
	}
	if hist[0].Detail != "confirmation page shown" {
		t.Errorf("detail = %q, want the validator summary", hist[0].Detail)
	}
}

func TestStore_AttemptHistory_TimestampsAreReal(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "timesite", db.StatusPending)
	if err := store.Settle("timesite", db.StatusFailed, true, "nope", "nope", ""); err != nil {
		t.Fatalf("settle: %v", err)
	}

	hist, err := store.AttemptHistory("timesite")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	// created_at is stored as text but the column is DECLARED TIMESTAMP, so
	// the driver converts it on the way out. Parsing that with the text layout
	// silently yields year 1 — wrong in a way that still prints and sorts.
	if hist[0].CreatedAt.IsZero() {
		t.Fatal("timestamp did not parse; every attempt would be dated year 1")
	}
	if hist[0].CreatedAt.Year() < 2000 {
		t.Errorf("timestamp year %d — the layout does not match what the driver returns", hist[0].CreatedAt.Year())
	}
}

func TestStore_AttemptHistory_IsAppendOnlyAndOrdered(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	seedOne(t, store, "multisite", db.StatusPending)

	for _, d := range []string{"first failure", "second failure"} {
		if err := store.Settle("multisite", db.StatusFailed, false, d, d, ""); err != nil {
			t.Fatalf("settle: %v", err)
		}
		if err := store.Reset("multisite"); err != nil {
			t.Fatalf("reset: %v", err)
		}
	}

	hist, err := store.AttemptHistory("multisite")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("got %d attempts, want 2 — history must survive a reset", len(hist))
	}
	if hist[0].Detail != "first failure" || hist[1].Detail != "second failure" {
		t.Errorf("history out of order or overwritten: %q then %q", hist[0].Detail, hist[1].Detail)
	}
}
