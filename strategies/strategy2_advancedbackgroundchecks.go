package strategies

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
)

// AdvancedBackgroundChecksOptOutURL is the Step-1 email-verification form.
//
// Verified live 2026-09-18, including a check that matters here specifically:
// this site shows a "protected by reCAPTCHA" badge (reCAPTCHA v3, score-based
// and passive — unlike the interactive challenges on FamilyTreeNow, USPhoneBook,
// Nuwber, and Clustal, which chromedp either got stuck on or which require an
// explicit solve). Confirmed chromedp reaches the real form here without
// getting stuck on a bot-check interstitial. What's NOT verified: whether a
// fresh, cookie-less chromedp session's reCAPTCHA v3 score is low enough to
// pass at actual submit time — that risk is real but distinct from "does the
// page load," which is what was checked.
const AdvancedBackgroundChecksOptOutURL = "https://www.advancedbackgroundchecks.com/opt-out"

// splitName splits "First Last" into ("First", "Last"). Extra words (a
// middle name, a suffix) are folded into the last-name field — this form
// has a separate middle-name field but Config doesn't collect one.
func splitName(full string) (first, last string) {
	parts := strings.Fields(full)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

func (s *Strategy2) runAdvancedBackgroundChecks(ctx context.Context, cfg *config.Config, dryRun bool) (agent.Result, error) {
	if err := agent.ValidateURL(AdvancedBackgroundChecksOptOutURL); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("allowlist: %w", err)
	}

	if dryRun {
		return agent.Result{
			Status: db.StatusPending,
			Notes:  fmt.Sprintf("DRY_RUN: would navigate to %s, submit name+email, and validate the confirmation response", AdvancedBackgroundChecksOptOutURL),
			DryRun: true,
		}, nil
	}

	if err := cfg.IsReady(); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("config not ready: %w", err)
	}

	return s.runAdvancedBackgroundChecksBrowser(ctx, cfg)
}

func (s *Strategy2) runAdvancedBackgroundChecksBrowser(ctx context.Context, cfg *config.Config) (agent.Result, error) {
	step := func(n, total int, msg string) {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", n, total, msg)
	}
	const steps = 4

	taskCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	first, last := splitName(cfg.SubjectName)

	step(1, steps, fmt.Sprintf("navigating → %s", AdvancedBackgroundChecksOptOutURL))
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(AdvancedBackgroundChecksOptOutURL),
		chromedp.WaitVisible(`#sfn`, chromedp.ByID),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("navigate: %w", err)
	}

	step(2, steps, "filling opt-out request form")
	// #mode ("I am") already defaults to "subject" — correct for this
	// tool's use case (the subject requesting their own removal), left alone.
	if err := chromedp.Run(taskCtx,
		chromedp.SendKeys(`#sfn`, first, chromedp.ByID),
		chromedp.SendKeys(`#sln`, last, chromedp.ByID),
		chromedp.SendKeys(`#semail`, cfg.SubjectEmail, chromedp.ByID),
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
	_ = os.WriteFile("/tmp/strategy2_advancedbackgroundchecks_post_submit.png", screenshot, 0600)

	var domText string
	_ = chromedp.Run(taskCtx, chromedp.Text(`body`, &domText, chromedp.ByQuery))

	step(4, steps, "Claude Haiku validating post-submit screenshot…")
	validator := agent.NewValidator(cfg.AnthropicKey)
	ok, summary, err := validator.ValidateOutcome(ctx, screenshot, "AdvancedBackgroundChecks", domText)
	if err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("validate: %w", err)
	}
	if !ok {
		return agent.Result{Status: db.StatusFailed, Notes: summary}, nil
	}
	return agent.Result{Status: db.StatusSuccess, Notes: summary}, nil
}
