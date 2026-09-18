package strategies

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// CheckPeopleOptOutURL is CheckPeople's Suppression Center entry point.
// Verified live 2026-09-17: a same-origin, single-page email-request form —
// no cookie banner, no iframe. Submitting it sends a confirmation email the
// subject still has to click themselves; that's treated as success below,
// matching ValidateOutcome's existing needs_email_confirmation semantics.
const CheckPeopleOptOutURL = "https://checkpeople.com/opt-out"

// Strategy2 handles Strategy 2 (Hidden Opt-Out Pages) brokers.
//
// Unlike Strategy 1, this bucket has no single shared mechanism across all
// its brokers — every site needs its own verified selectors, the same way
// Strategy 1 needed its own for TruthFinder (cookie banners, iframes, and
// multi-step disclosure vary site to site; a generic vision-only pass isn't
// reliable enough to trust unattended). Sites are added to this switch one
// at a time, only after being checked live. Any broker.ID not yet added is
// routed to StatusManual rather than attempted with unverified selectors —
// see docs/SECURITY-BASELINE.md 2026-09-17 audit entry for why.
type Strategy2 struct{}

func NewStrategy2() *Strategy2 { return &Strategy2{} }

func (s *Strategy2) Run(
	ctx context.Context,
	broker db.Broker,
	cfg *config.Config,
	dryRun bool,
) (agent.Result, error) {
	switch broker.ID {
	case "checkpeople":
		return s.runCheckPeople(ctx, cfg, dryRun)
	case "advancedbackgroundchecks":
		return s.runAdvancedBackgroundChecks(ctx, cfg, dryRun)
	default:
		// Dry-run must never change persisted state — matches Strategy 1's
		// dry-run gate, which always reports StatusPending so the broker
		// stays dispatchable. Only a real run settles it to StatusManual.
		if dryRun {
			return agent.Result{
				Status: db.StatusPending,
				DryRun: true,
				Notes:  fmt.Sprintf("DRY_RUN: %s has no verified Strategy 2 handler yet — a real run would mark it manual", broker.Name),
			}, nil
		}
		return agent.Result{
			Status: db.StatusManual,
			Notes:  fmt.Sprintf("no verified Strategy 2 handler for %s yet — use the manual opt-out checklist", broker.Name),
		}, nil
	}
}

func (s *Strategy2) runCheckPeople(ctx context.Context, cfg *config.Config, dryRun bool) (agent.Result, error) {
	// ── Gate 1: Allowlist ──────────────────────────────────────────────
	if err := agent.ValidateURL(CheckPeopleOptOutURL); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("allowlist: %w", err)
	}

	// ── Gate 2: Dry-run ────────────────────────────────────────────────
	if dryRun {
		return agent.Result{
			Status: db.StatusPending,
			Notes:  fmt.Sprintf("DRY_RUN: would navigate to %s, submit the email opt-out request, and validate the confirmation response", CheckPeopleOptOutURL),
			DryRun: true,
		}, nil
	}

	// ── Gate 3: Config ────────────────────────────────────────────────
	if err := cfg.IsReady(); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("config not ready: %w", err)
	}

	return s.runBrowser(ctx, cfg)
}

func (s *Strategy2) runBrowser(ctx context.Context, cfg *config.Config) (agent.Result, error) {
	step := func(n, total int, msg string) {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", n, total, msg)
	}

	taskCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	const total = 4

	step(1, total, fmt.Sprintf("Navigating to %s", CheckPeopleOptOutURL))
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(CheckPeopleOptOutURL),
		chromedp.WaitVisible(`#requestorEmail`, chromedp.ByID),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("navigate: %w", err)
	}

	step(2, total, "Filling opt-out request form")
	if err := chromedp.Run(taskCtx,
		chromedp.SendKeys(`#requestorEmail`, cfg.SubjectEmail, chromedp.ByID),
		chromedp.Click(`#acknowledge`, chromedp.ByID),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("fill form: %w", err)
	}

	step(3, total, "Submitting request")
	var screenshot []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Click(`button[type="submit"].cp-auto-optout__button`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.CaptureScreenshot(&screenshot),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("submit: %w", err)
	}
	_ = os.WriteFile("/tmp/strategy2_checkpeople_post_submit.png", screenshot, 0600)

	var domText string
	_ = chromedp.Run(taskCtx, chromedp.Text(`body`, &domText, chromedp.ByQuery))

	step(4, total, "Claude Haiku validating post-submit screenshot…")
	validator := agent.NewValidator(cfg.AnthropicKey)
	ok, summary, err := validator.ValidateOutcome(ctx, screenshot, "CheckPeople", domText)
	if err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("validate: %w", err)
	}
	if !ok {
		return agent.Result{Status: db.StatusFailed, Notes: summary}, nil
	}
	return agent.Result{Status: db.StatusSuccess, Notes: summary}, nil
}
