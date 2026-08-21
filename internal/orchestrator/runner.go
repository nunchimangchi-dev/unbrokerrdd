// Package orchestrator manages batch execution of opt-out agents.
// It enforces idempotency, rate limiting, dry-run mode, and QA gates.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// BatchConfig controls how a batch run behaves.
type BatchConfig struct {
	Strategy int           // which strategy (1-6) to run
	DryRun   bool          // if true: full execution path but no form submission
	Limit    int           // max brokers to process (0 = all pending)
	Delay    time.Duration // minimum gap between broker dispatches (rate limiting)
}

// StatusUpdate is broadcast to the dashboard after each broker completes.
type StatusUpdate struct {
	BrokerID string
	Status   db.Status
	DryRun   bool
	Notes    string
}

// Runner executes batches against the SQLite store.
type Runner struct {
	store    *db.Store
	cfg      *config.Config
	agents   map[int]agent.Agent    // strategy number → agent implementation
	notify   func(StatusUpdate)     // called after each broker settles (dashboard hook)
}

// New creates a Runner. agents maps strategy numbers to their implementations.
// notify is called on the goroutine that runs the agent — keep it non-blocking.
func New(store *db.Store, cfg *config.Config, agents map[int]agent.Agent, notify func(StatusUpdate)) *Runner {
	return &Runner{store: store, cfg: cfg, agents: agents, notify: notify}
}

// RunBatch executes all pending brokers for the given strategy.
// It will not proceed unless cfg.IsReady() passes (when not dry-running).
//
// Security contracts enforced here:
//  1. IsReady() — config validation before any live run
//  2. CanDispatch() — idempotency: never dispatch a non-pending broker
//  3. rate limiting — min Delay between dispatches
//  4. DryRun flag — full code path, no actual submission
func (r *Runner) RunBatch(ctx context.Context, bc BatchConfig) error {
	// Config gate: require full .env for live runs
	if !bc.DryRun {
		if err := r.cfg.IsReady(); err != nil {
			return fmt.Errorf("LIVE RUN BLOCKED — %w\n"+
				"Run with --dry-run to test without credentials", err)
		}
	}

	// Resolve agent
	a, ok := r.agents[bc.Strategy]
	if !ok {
		return fmt.Errorf("no agent registered for strategy %d", bc.Strategy)
	}

	// Load pending brokers
	brokers, err := r.store.GetPendingByStrategy(bc.Strategy)
	if err != nil {
		return fmt.Errorf("load pending brokers: %w", err)
	}
	if len(brokers) == 0 {
		log.Printf("[orchestrator] strategy %d: no pending brokers", bc.Strategy)
		return nil
	}

	// Apply limit
	if bc.Limit > 0 && len(brokers) > bc.Limit {
		brokers = brokers[:bc.Limit]
	}

	mode := "LIVE"
	if bc.DryRun {
		mode = "DRY_RUN"
	}
	log.Printf("[orchestrator] strategy %d [%s]: dispatching %d brokers (delay=%s)",
		bc.Strategy, mode, len(brokers), bc.Delay)

	for i, b := range brokers {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// ── Idempotency guard ──────────────────────────────────────────
		ok, err := r.store.CanDispatch(b.ID)
		if err != nil {
			log.Printf("[orchestrator] %s: CanDispatch error: %v — skipping", b.ID, err)
			continue
		}
		if !ok {
			log.Printf("[orchestrator] %s: not pending — skipping (idempotency)", b.ID)
			continue
		}

		// ── Mark in-progress ──────────────────────────────────────────
		if err := r.store.SetInProgress(b.ID); err != nil {
			log.Printf("[orchestrator] %s: SetInProgress: %v — skipping", b.ID, err)
			continue
		}
		r.notify(StatusUpdate{BrokerID: b.ID, Status: db.StatusInProgress, DryRun: bc.DryRun})

		// ── Dispatch agent ────────────────────────────────────────────
		result, runErr := a.Run(ctx, b, r.cfg, bc.DryRun)

		finalStatus := result.Status
		notes := result.Notes
		if runErr != nil {
			finalStatus = db.StatusFailed
			notes = fmt.Sprintf("agent error: %v", runErr)
			log.Printf("[orchestrator] %s: agent error: %v", b.ID, runErr)
		}

		// ── Settle ────────────────────────────────────────────────────
		if err := r.store.Settle(b.ID, finalStatus, bc.DryRun, string(finalStatus), notes, result.ConfirmationURL); err != nil {
			log.Printf("[orchestrator] %s: Settle error: %v", b.ID, err)
		}
		r.notify(StatusUpdate{BrokerID: b.ID, Status: finalStatus, DryRun: bc.DryRun, Notes: notes})
		log.Printf("[orchestrator] %s → %s (dry_run=%v)", b.ID, finalStatus, bc.DryRun)

		// ── Cascade ───────────────────────────────────────────────────
		// Strategy 1: one TruthFinder submission covers all 7 affiliates.
		// Mark sibling IDs with the same outcome so we don't re-submit.
		for _, cascadeID := range result.CascadeIDs {
			cascadeNotes := fmt.Sprintf("cascade from %s: %s", b.ID, notes)
			if ok, _ := r.store.CanDispatch(cascadeID); ok {
				_ = r.store.SetInProgress(cascadeID)
				r.notify(StatusUpdate{BrokerID: cascadeID, Status: db.StatusInProgress, DryRun: bc.DryRun})
				if err := r.store.Settle(cascadeID, finalStatus, bc.DryRun, string(finalStatus), cascadeNotes, result.ConfirmationURL); err != nil {
					log.Printf("[orchestrator] cascade %s: Settle error: %v", cascadeID, err)
				} else {
					r.notify(StatusUpdate{BrokerID: cascadeID, Status: finalStatus, DryRun: bc.DryRun, Notes: cascadeNotes})
					log.Printf("[orchestrator] cascade %s → %s", cascadeID, finalStatus)
				}
			}
		}

		// ── Rate limiting ─────────────────────────────────────────────
		// Always wait between dispatches — last broker doesn't need a delay.
		if i < len(brokers)-1 {
			delay := bc.Delay
			if delay == 0 {
				delay = 5 * time.Second // hard minimum — never removed
			}
			log.Printf("[orchestrator] rate-limit delay: %s", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	log.Printf("[orchestrator] strategy %d [%s]: batch complete", bc.Strategy, mode)
	return nil
}
