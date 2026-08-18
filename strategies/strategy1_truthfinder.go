// Package strategies contains one file per opt-out strategy.
package strategies

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"databrokergo/internal/agent"
	"databrokergo/internal/config"
	"databrokergo/internal/db"
)

// TruthFinderOptOutURL is the single submission point that covers all 7 affiliates.
const TruthFinderOptOutURL = "https://www.truthfinder.com/privacy-center"

// strategy1AffiliateIDs are all 7 broker IDs in the TruthFinder cascade.
// One form submission at truthfinder.com/privacy-center covers all of them.
// When the orchestrator processes the first one, it cascade-settles the rest.
var strategy1AffiliateIDs = []string{
	"backgroundcheckme",
	"newyorkpublicrecords",
	"oregonpublicrecords",
	"publicrecordscenter",
	"publicrecordsreviews",
	"publicsrecords",
	"top4backgroundchecks",
}

// Strategy1TruthFinder handles the TruthFinder affiliate cascade opt-out.
// One submission cascades deletion across 7 affiliated sites.
//
// Execution model:
//  1. chromedp opens a VISIBLE Chrome window (for QA oversight)
//  2. Navigate → expand accordion → fill #deletionEmail immediately (no Haiku round-trip —
//     the accordion auto-collapses after ~10s; Haiku detection takes 15-20s, too slow)
//  3. Click "Delete My User Data" → handle Opt-out Preferences modal (Save preferences)
//  4. Claude Haiku (vision) validates the outcome from the post-submit screenshot
//  5. On success: CascadeIDs returned so orchestrator marks all 7 affiliates
type Strategy1TruthFinder struct{}

func NewStrategy1() *Strategy1TruthFinder { return &Strategy1TruthFinder{} }

func (s *Strategy1TruthFinder) Run(
	ctx context.Context,
	broker db.Broker,
	cfg *config.Config,
	dryRun bool,
) (agent.Result, error) {

	// ── Gate 1: Allowlist ──────────────────────────────────────────────
	if err := agent.ValidateURL(TruthFinderOptOutURL); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("allowlist: %w", err)
	}

	// ── Gate 2: Dry-run ────────────────────────────────────────────────
	if dryRun {
		log.Printf("[strategy1] DRY_RUN: would navigate to %s and submit opt-out", TruthFinderOptOutURL)
		return agent.Result{
			Status: db.StatusPending,
			Notes:  fmt.Sprintf("DRY_RUN: would navigate to %s and submit opt-out form for strategy-1 cascade", TruthFinderOptOutURL),
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

	// ── Chrome setup (VISIBLE — not headless, for QA) ─────────────────
	step(1, steps, "launching Chrome (visible window)…")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.WindowSize(1280, 900),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(log.Printf),
	)
	defer cancelTask()

	// Hard timeout: 3 minutes. Without the Haiku form-detection round-trip
	// (saved ~20s) the flow should complete well within this.
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 3*time.Minute)
	defer cancelTimeout()

	// ── Navigate + expand accordion + fill immediately ────────────────
	// The "Right to Delete" accordion auto-collapses after ~10 seconds.
	// We must navigate → expand → fill as one tight sequence without any
	// API calls in between.
	step(2, steps, fmt.Sprintf("navigating → %s", TruthFinderOptOutURL))

	// ── Navigate ──────────────────────────────────────────────────────────
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(TruthFinderOptOutURL),
		chromedp.Sleep(4*time.Second),
	); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("navigate: %w", err)
	}
	var afterNavShot []byte
	_ = chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&afterNavShot))
	_ = os.WriteFile("/tmp/strategy1_after_nav.png", afterNavShot, 0600)

	// ── Dismiss initial cookie / opt-out modal ────────────────────────────
	// TruthFinder shows the "Opt-out Preferences" modal on page load.
	// It covers the form and blocks all chromedp clicks.
	// "Save My Preferences" = acknowledge cookie prefs and continue to the page.
	// "Cancel" would navigate away — never click Cancel.
	fmt.Fprintf(os.Stderr, "  → dismissing initial opt-out modal (if any)…\n")
	handleOptOutModal(taskCtx)
	if err := chromedp.Run(taskCtx, chromedp.Sleep(2*time.Second)); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("post-modal sleep: %w", err)
	}
	var afterModalShot []byte
	_ = chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&afterModalShot))
	_ = os.WriteFile("/tmp/strategy1_after_modal.png", afterModalShot, 0600)

	// ── Scroll to + fill email input ──────────────────────────────────────
	// #deletionEmail is the Section A email input. It may or may not require
	// expanding an accordion first. We scroll it into view before clicking.
	fmt.Fprintf(os.Stderr, "  → scrolling to #deletionEmail and filling…\n")

	// Scroll element into view via JS (doesn't require visibility).
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(`
		(function() {
			var el = document.querySelector('#deletionEmail');
			if (el) { el.scrollIntoView({block:'center'}); return true; }
			return false;
		})()
	`, nil))
	if err := chromedp.Run(taskCtx, chromedp.Sleep(500*time.Millisecond)); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("scroll sleep: %w", err)
	}

	// Fill using native React setter + real SendKeys for keyboard events.
	fillScript := fmt.Sprintf(`
		(function() {
			var inp = document.querySelector('#deletionEmail') ||
			          document.querySelector('input[type="email"]');
			if (!inp) return 'not-found';
			inp.scrollIntoView({block:'center'});
			inp.focus();
			var nativeSetter = Object.getOwnPropertyDescriptor(
				window.HTMLInputElement.prototype, 'value').set;
			nativeSetter.call(inp, %q);
			inp.dispatchEvent(new Event('input',  {bubbles:true}));
			inp.dispatchEvent(new Event('change', {bubbles:true}));
			return 'filled';
		})()
	`, cfg.SubjectEmail)
	var fillResult string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(fillScript, &fillResult))
	fmt.Fprintf(os.Stderr, "  → email JS fill: %s\n", fillResult)

	// SendKeys fires real keyboard events React needs for validation.
	// Use WaitVisible with a short timeout so we fail fast if element is truly gone.
	sendKeysCtx, cancelSendKeys := context.WithTimeout(taskCtx, 10*time.Second)
	defer cancelSendKeys()
	emailSel := "#deletionEmail"
	if err := chromedp.Run(sendKeysCtx,
		chromedp.WaitVisible(emailSel, chromedp.ByQuery),
		chromedp.Click(emailSel, chromedp.ByQuery),
		chromedp.SendKeys(emailSel, cfg.SubjectEmail, chromedp.ByQuery),
	); err != nil {
		// Non-fatal: JS fill already set the value; log and continue.
		fmt.Fprintf(os.Stderr, "  ⚠ SendKeys failed (%v) — JS fill only\n", err)
	}

	fmt.Fprintf(os.Stderr, "  ✓ email fill attempted (JS + SendKeys)\n")

	var afterFillShot []byte
	_ = chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&afterFillShot))
	_ = os.WriteFile("/tmp/strategy1_after_fill.png", afterFillShot, 0600)

	// ── Operator abort window ─────────────────────────────────────────
	fmt.Fprintf(os.Stderr, "  → submitting in 3 seconds (close browser to abort)…\n")
	time.Sleep(3 * time.Second)

	// ── Submit ────────────────────────────────────────────────────────
	// Use real chromedp.Click which fires proper MouseEvents.
	// The accordion should still be expanded since we filled quickly.
	step(3, steps, "clicking 'Delete My User Data' + handling modal…")
	submitXPath := `//button[contains(., 'Delete My User Data')]`

	// Take a pre-submit debug screenshot.
	var preShot []byte
	_ = chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&preShot))
	_ = os.WriteFile("/tmp/strategy1_pre_submit.png", preShot, 0600)

	if err := chromedp.Run(taskCtx,
		chromedp.Click(submitXPath, chromedp.BySearch),
		chromedp.Sleep(3*time.Second), // wait for modal / AJAX response
	); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ submit click failed (%v)\n", err)
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("click submit: %w", err)
	}

	// Handle the "Opt-out Preferences" cookie modal that TruthFinder shows
	// after clicking "Delete My User Data".
	// "Save My Preferences" acknowledges cookie preferences and lets deletion proceed.
	handleOptOutModal(taskCtx)
	if err := chromedp.Run(taskCtx, chromedp.Sleep(2*time.Second)); err != nil {
		return agent.Result{Status: db.StatusFailed}, fmt.Errorf("post-modal wait: %w", err)
	}

	// ── Screenshot for validation ─────────────────────────────────────
	var screenshot []byte
	if err := chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&screenshot)); err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("post-submit screenshot: %w", err)
	}
	_ = os.WriteFile("/tmp/strategy1_post_submit.png", screenshot, 0600)

	// Quick DOM-wide text scan for success phrases — catches inline confirmations
	// and toast notifications.  Also passed to Haiku as additional context.
	var domText string
	_ = chromedp.Run(taskCtx, chromedp.Evaluate(
		`document.body ? document.body.innerText.substring(0, 2000) : ''`, &domText,
	))
	domLower := strings.ToLower(domText)
	successPhrases := []string{
		"check your email", "email sent", "request submitted",
		"will be removed", "we received", "email to confirm",
		"confirmation", "deletion request", "successfully",
	}
	for _, p := range successPhrases {
		if strings.Contains(domLower, p) {
			fmt.Fprintf(os.Stderr, "  ✓ DOM success phrase: %q\n", p)
			break
		}
	}

	// ── Claude Haiku: validate outcome ────────────────────────────────
	// Pass domText so Haiku can see toast notifications / inline text even if
	// they are hard to read in the screenshot.
	step(4, steps, "Claude Haiku validating post-submit screenshot…")
	validator := agent.NewValidator(cfg.AnthropicKey)
	ok, summary, err := validator.ValidateOutcome(ctx, screenshot, "TruthFinder", domText)
	if err != nil {
		return agent.Result{Status: db.StatusFailed},
			fmt.Errorf("haiku outcome validation: %w", err)
	}

	if !ok {
		fmt.Fprintf(os.Stderr, "  ✗ submission did not appear to succeed: %s\n", summary)
		return agent.Result{
			Status: db.StatusFailed,
			Notes:  "Haiku: submission did not appear to succeed — " + summary,
		}, nil
	}

	fmt.Fprintf(os.Stderr, "  ✓ submission confirmed: %s\n", summary)
	fmt.Fprintf(os.Stderr, "  ↳ cascading success to %d affiliate brokers\n", len(strategy1AffiliateIDs)-1)

	return agent.Result{
		Status:     db.StatusSuccess,
		Notes:      summary,
		CascadeIDs: s.cascadeIDs(broker.ID),
	}, nil
}

// handleOptOutModal handles the "Opt-out Preferences" cookie consent gate that
// TruthFinder shows after clicking "Delete My User Data".
// Clicks "Save My Preferences" — the path that lets deletion proceed.
// Clicking "Cancel" aborts the deletion and returns to the landing page.
// Safe to call even when no modal is visible.
func handleOptOutModal(ctx context.Context) {
	script := `
		(function() {
			var btns = Array.from(document.querySelectorAll('button'));
			var modalTitle = Array.from(document.querySelectorAll('*'))
				.find(function(el) {
					return el.children.length === 0 &&
					       el.textContent.trim().toLowerCase().includes('opt-out');
				});
			if (!modalTitle) return 'no modal';

			// Check "Do Not Sell or Share My Personal Information" if unchecked
			var checkbox = document.querySelector('input[type="checkbox"]');
			if (checkbox && !checkbox.checked) checkbox.click();

			// Click "Save My Preferences"
			var save = btns.find(function(b) {
				var t = b.textContent.trim().toLowerCase();
				return t.includes('save') || t.includes('accept') || t.includes('confirm');
			});
			if (save) { save.click(); return 'saved'; }

			// Fallback: × close button
			var close = document.querySelector('[aria-label="Close"], [title="Close"]');
			if (close) { close.click(); return 'closed'; }

			return 'modal-but-no-action';
		})();
	`
	_ = chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// chromedpClick fires a click on a selector via JS — returns immediately
// without waiting for navigation.
func chromedpClick(ctx context.Context, sel string) error {
	if strings.Contains(sel, ":contains(") {
		sel = containsToXPath(sel)
		return chromedp.Run(ctx, chromedp.Click(sel, chromedp.BySearch))
	}
	script := fmt.Sprintf(`
		(function() {
			var all = Array.from(document.querySelectorAll('button, input[type="submit"]'));
			var strong = ['delete my user data','remove my data','opt out','submit request'];
			var el = all.find(function(b) {
				var t = (b.textContent || b.value || '').toLowerCase().trim();
				return strong.some(function(w) { return t === w || t.startsWith(w); });
			});
			if (!el) {
				var weak = ['delete','remove','opt-out','submit','send','request'];
				el = all.find(function(b) {
					var t = (b.textContent || b.value || '').toLowerCase().trim();
					return weak.some(function(w) { return t.includes(w); });
				});
			}
			if (!el) el = document.querySelector(%q);
			if (!el) el = document.querySelector('button[type="submit"]');
			if (!el) throw new Error('no submit button found for: ' + %q);
			el.click();
		})();
	`, sel, sel)
	return chromedp.Run(ctx, chromedp.Evaluate(script, nil))
}

// containsToXPath converts jQuery :contains() to XPath.
func containsToXPath(sel string) string {
	idx := strings.Index(sel, ":contains(")
	if idx < 0 {
		return "//" + sel
	}
	tag := sel[:idx]
	rest := strings.TrimRight(sel[idx+len(":contains("):], ")")
	rest = strings.Trim(rest, `'"`)
	if tag == "" {
		tag = "*"
	}
	return fmt.Sprintf("//%s[contains(.,'%s')]", tag, rest)
}

// splitName splits "First Last" into ("First", "Last").
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
