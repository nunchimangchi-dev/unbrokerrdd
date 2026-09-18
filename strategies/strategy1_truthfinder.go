// Package strategies contains one file per opt-out strategy.
package strategies

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// TruthFinderSuppressionURL is PeopleConnect's shared suppression portal.
//
// CORRECTED 2026-09-17: this used to point at truthfinder.com/privacy-center's
// "Delete My User Data" button, which TruthFinder's own page states explicitly
// does NOT suppress your Background Report from being found in searches — it
// only deletes account/visitor data. That was verified live and was wrong for
// this tool's entire purpose. This URL is the actual suppression flow, reached
// from TruthFinder's own "Suppress Your Background Report" link.
const TruthFinderSuppressionURL = "https://suppression.peopleconnect.us/?brand=TruthFinder"

// strategy1AffiliateIDs are the 7 broker IDs this submission is assumed to cascade-settle.
//
// CAVEAT (unresolved, not newly introduced by the 2026-09-17 fix): this list
// and the cascade assumption itself predate this fix and were never
// independently verified against the corrected (or the original) flow — they
// were inherited from earlier AI-assisted project work. Treat cascade
// successes as an unconfirmed assumption, not a verified fact, until someone
// checks a couple of these 7 sites directly after a real submission.
var strategy1AffiliateIDs = []string{
	"backgroundcheckme",
	"newyorkpublicrecords",
	"oregonpublicrecords",
	"publicrecordscenter",
	"publicrecordsreviews",
	"publicsrecords",
	"top4backgroundchecks",
}

// Strategy1TruthFinder handles the PeopleConnect suppression submission that
// (assumed — see caveat above) cascades across the 7 TruthFinder-affiliate
// brokers.
//
// Execution model:
//  1. chromedp opens a VISIBLE Chrome window (for QA oversight)
//  2. Navigate to the suppression portal, fill email, check consent, submit
//  3. Claude Haiku (vision) validates the outcome from the post-submit screenshot
//  4. On success: CascadeIDs returned so orchestrator marks all 7 affiliates
//
// This is Step 1 of PeopleConnect's flow only (email → verification link).
// The subject still has to click the confirmation email themselves, and the
// portal may ask for further identity verification (address, phone, prior
// emails) before actually locating and suppressing every listing — this tool
// doesn't collect that data (deliberate minimal-data principle) and can't
// automate past this point. A "success" here means the request was
// submitted, not that suppression is fully complete.
type Strategy1TruthFinder struct{}

func NewStrategy1() *Strategy1TruthFinder { return &Strategy1TruthFinder{} }

func (s *Strategy1TruthFinder) Run(
	ctx context.Context,
	broker db.Broker,
	cfg *config.Config,
	dryRun bool,
) (agent.Result, error) {

	// ── Gate 1: Allowlist ──────────────────────────────────────────────
	if err := agent.ValidateURL(TruthFinderSuppressionURL); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("allowlist: %w", err)
	}

	// ── Gate 2: Dry-run ────────────────────────────────────────────────
	if dryRun {
		log.Printf("[strategy1] DRY_RUN: would navigate to %s and submit the suppression request", TruthFinderSuppressionURL)
		return agent.Result{
			Status: db.StatusPending,
			Notes:  fmt.Sprintf("DRY_RUN: would navigate to %s, submit the suppression request, and validate the confirmation response", TruthFinderSuppressionURL),
			DryRun: true,
		}, nil
	}

	// ── Gate 3: Config ────────────────────────────────────────────────
	if err := cfg.IsReady(); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("config not ready: %w", err)
	}

	return s.runBrowser(ctx, broker, cfg)
}

// cascadeIDs returns the other 6 affiliate IDs to cascade-settle after
// the one that ran the browser submission.
func (s *Strategy1TruthFinder) cascadeIDs(submittedID string) []string {
	out := make([]string, 0, len(strategy1AffiliateIDs)-1)
	for _, id := range strategy1AffiliateIDs {
		if id != submittedID {
			out = append(out, id)
		}
	}
	return out
}

func (s *Strategy1TruthFinder) runBrowser(ctx context.Context, broker db.Broker, cfg *config.Config) (agent.Result, error) {
	step := func(n, total int, msg string) {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", n, total, msg)
	}
	const steps = 4

	taskCtx, cancel := chromedp.NewContext(ctx, chromedp.WithLogf(log.Printf))
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	step(1, steps, fmt.Sprintf("navigating → %s", TruthFinderSuppressionURL))
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(TruthFinderSuppressionURL),
		chromedp.WaitVisible(`input[name="login-email"]`, chromedp.ByQuery),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("navigate: %w", err)
	}

	step(2, steps, "filling suppression request form")
	if err := chromedp.Run(taskCtx,
		chromedp.SendKeys(`input[name="login-email"]`, cfg.SubjectEmail, chromedp.ByQuery),
		chromedp.Click(`input[name="consent"]`, chromedp.ByQuery),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("fill form: %w", err)
	}

	step(3, steps, "submitting request")
	var screenshot []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.CaptureScreenshot(&screenshot),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("submit: %w", err)
	}
	_ = os.WriteFile("/tmp/strategy1_post_submit.png", screenshot, 0600)

	var domText string
	_ = chromedp.Run(taskCtx, chromedp.Text(`body`, &domText, chromedp.ByQuery))

	step(4, steps, "Claude Haiku validating post-submit screenshot…")
	validator := agent.NewValidator(cfg.AnthropicKey)
	ok, summary, err := validator.ValidateOutcome(ctx, screenshot, "TruthFinder (PeopleConnect Suppression Center)", domText)
	if err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("haiku outcome validation: %w", err)
	}

	if !ok {
		fmt.Fprintf(os.Stderr, "  ✗ submission did not appear to succeed: %s\n", summary)
		return agent.Result{
			Status: db.StatusFailed,
			Notes:  "Haiku: submission did not appear to succeed — " + summary,
		}, nil
	}

	fmt.Fprintf(os.Stderr, "  ✓ submission confirmed: %s\n", summary)
	fmt.Fprintf(os.Stderr, "  ↳ cascading (unverified assumption — see doc comment) to %d affiliate brokers\n", len(strategy1AffiliateIDs)-1)

	return agent.Result{
		Status:     db.StatusSuccess,
		Notes:      summary,
		CascadeIDs: s.cascadeIDs(broker.ID),
	}, nil
}
