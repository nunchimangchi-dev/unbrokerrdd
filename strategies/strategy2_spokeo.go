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

// SpokeoOptOutURL is Spokeo's opt-out form.
//
// Verified live 2026-09-18: no CAPTCHA/Turnstile/Cloudflare-challenge
// anywhere on the page (checked via direct DOM query, not assumed) - one of
// only 1 of 5 ProfileURL-needing sites checked that day that turned out
// genuinely clean; the other 4 (Alumnius, BeenVerified, SmartBackgroundChecks,
// Clustal) are all bot-defended despite also needing a profile URL. Form is a
// distinct `.optout_container` (a stable semantic class, not a CSS-in-JS
// hash) with exactly two real inputs - `input[type="url"]` and
// `input[type="email"]` - separate from the page's unrelated name/phone/
// address SEARCH form, which is why selectors are scoped to that container
// rather than a bare `input[type="url"]` query.
const SpokeoOptOutURL = "https://www.spokeo.com/optout"

// runSpokeo requires broker.ProfileURL to already be set (via
// `databrokergo set-profile-url`) - this is the search-and-select-your-
// listing pattern: Spokeo has no way to identify which record is the
// subject's from name/email alone, and guessing risks opting out a
// stranger's real listing without their consent.
func (s *Strategy2) runSpokeo(ctx context.Context, broker db.Broker, cfg *config.Config, dryRun bool) (agent.Result, error) {
	if err := agent.ValidateURL(SpokeoOptOutURL); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("allowlist: %w", err)
	}

	if broker.ProfileURL == "" {
		if dryRun {
			return agent.Result{
				Status: db.StatusPending,
				DryRun: true,
				Notes:  "DRY_RUN: no profile_url set yet - would otherwise navigate, fill, and submit. Run `databrokergo set-profile-url --broker spokeo --url <your-listing-url>` first.",
			}, nil
		}
		return agent.Result{
			Status: db.StatusManual,
			Notes:  "No profile_url set - find your own Spokeo listing and run `databrokergo set-profile-url --broker spokeo --url <your-listing-url>` before retrying.",
		}, nil
	}

	if dryRun {
		return agent.Result{
			Status: db.StatusPending,
			DryRun: true,
			Notes:  fmt.Sprintf("DRY_RUN: would navigate to %s, submit profile_url + email, and validate the confirmation response", SpokeoOptOutURL),
		}, nil
	}

	if err := cfg.IsReady(); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("config not ready: %w", err)
	}

	return s.runSpokeoBrowser(ctx, broker.ProfileURL, cfg)
}

func (s *Strategy2) runSpokeoBrowser(ctx context.Context, profileURL string, cfg *config.Config) (agent.Result, error) {
	step := func(n, total int, msg string) {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", n, total, msg)
	}
	const steps = 4

	taskCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	const urlSel = `.optout_container input[type="url"]`
	const emailSel = `.optout_container input[type="email"]`
	const submitSel = `.optout_container button[type="submit"]`

	step(1, steps, fmt.Sprintf("navigating → %s", SpokeoOptOutURL))
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(SpokeoOptOutURL),
		chromedp.WaitVisible(urlSel, chromedp.ByQuery),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("navigate: %w", err)
	}

	step(2, steps, "filling opt-out request form (profile URL + email)")
	if err := chromedp.Run(taskCtx,
		chromedp.SendKeys(urlSel, profileURL, chromedp.ByQuery),
		chromedp.SendKeys(emailSel, cfg.SubjectEmail, chromedp.ByQuery),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("fill form: %w", err)
	}

	step(3, steps, "submitting request")
	var screenshot []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Click(submitSel, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.CaptureScreenshot(&screenshot),
	); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("submit: %w", err)
	}
	_ = os.WriteFile("/tmp/strategy2_spokeo_post_submit.png", screenshot, 0600)

	var domText string
	_ = chromedp.Run(taskCtx, chromedp.Text(`body`, &domText, chromedp.ByQuery))

	step(4, steps, "Claude Haiku validating post-submit screenshot…")
	validator := agent.NewValidator(cfg.AnthropicKey)
	ok, summary, err := validator.ValidateOutcome(ctx, screenshot, "Spokeo", domText)
	if err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("validate: %w", err)
	}
	if !ok {
		return agent.Result{Status: db.StatusFailed, Notes: summary}, nil
	}
	return agent.Result{Status: db.StatusSuccess, Notes: summary}, nil
}
