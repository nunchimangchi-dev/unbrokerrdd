package agent

import (
	"context"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// Result is returned by every strategy agent after processing a broker.
type Result struct {
	Status          db.Status // final status to write to SQLite
	ConfirmationURL string    // URL of the confirmation page, if any
	Notes           string    // human-readable summary (safe to log — no PII)
	DryRun          bool      // true if no real submission was made

	// CascadeIDs lists sibling broker IDs that should be settled with the
	// same status without re-dispatching the agent.  Used by Strategy 1:
	// one TruthFinder submission covers all 7 affiliate sites.
	CascadeIDs []string
}

// Agent defines the interface every strategy implementation must satisfy.
// All agents must:
//   - Call agent.ValidateURL before any navigation
//   - Respect dryRun — return early without submitting if true
//   - Never log personal data from cfg
//   - Be idempotent — the orchestrator guarantees single dispatch,
//     but agents should handle re-entry gracefully
type Agent interface {
	Run(ctx context.Context, broker db.Broker, cfg *config.Config, dryRun bool) (Result, error)
}
