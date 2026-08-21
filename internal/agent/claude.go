package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// imgMIME detects whether img is PNG or JPEG from its magic bytes.
// chromedp.CaptureScreenshot can return either format depending on CDP flags;
// sending the wrong MIME type causes a 400 from the Anthropic API.
func imgMIME(img []byte) string {
	if len(img) >= 4 &&
		img[0] == 0x89 && img[1] == 0x50 && img[2] == 0x4E && img[3] == 0x47 {
		return "image/png"
	}
	return "image/jpeg" // JPEG magic is 0xFF 0xD8 — everything else treated as JPEG
}

// Validator uses Claude Haiku with vision to inspect browser screenshots.
// It makes two decisions no deterministic script can make:
//   - "where is the opt-out form and what are the field selectors?"
//   - "did the form submission actually succeed?"
//
// Model: claude-haiku-4-5-20251001 — fast, cheap, adequate for binary visual decisions.
type Validator struct {
	client anthropic.Client // value type, not pointer
}

// NewValidator creates a Haiku-backed screenshot validator.
func NewValidator(apiKey string) *Validator {
	return &Validator{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// FormSelectors is the structured response Claude returns after analysing a page.
type FormSelectors struct {
	FirstName  string `json:"first_name"`  // CSS selector or empty
	LastName   string `json:"last_name"`   // CSS selector or empty
	Email      string `json:"email"`       // CSS selector
	State      string `json:"state"`       // CSS selector or empty
	Submit     string `json:"submit"`      // CSS selector for the submit button
	HasCAPTCHA bool   `json:"has_captcha"` // true if CAPTCHA is visible
	Notes      string `json:"notes"`       // human-readable description
}

// FindOptOutForm sends a screenshot to Claude Haiku and asks it to locate
// the CSS selectors for an opt-out / data removal form.
// Returns nil if no form is found on the page.
func (v *Validator) FindOptOutForm(ctx context.Context, screenshotPNG []byte) (*FormSelectors, error) {
	mime   := imgMIME(screenshotPNG)
	imgB64 := base64.StdEncoding.EncodeToString(screenshotPNG)

	system := anthropic.TextBlockParam{
		Text: `You are an expert at identifying data broker opt-out and privacy request forms.
Analyse the browser screenshot and return CSS selectors for the form fields.
Respond with valid JSON only — no markdown fences, no explanation.
If no opt-out form is visible, return {"notes":"NO_FORM_FOUND"}.
If a CAPTCHA (reCAPTCHA, hCaptcha, slider puzzle) is visible, set has_captcha:true.

CRITICAL selector rules — violating these causes runtime errors:
- Use ONLY standard CSS Level 3 selectors that work with document.querySelector()
- NEVER use :contains(), :has-text(), :text(), or any jQuery pseudo-selectors
- For buttons with known text, use: button[type="submit"], input[type="submit"], or just button
- For email fields, prefer: input[type="email"], then input[name="email"], then input[type="text"]
- If multiple elements match a general selector, add an attribute to narrow it (e.g. input[type="email"])
- Empty string is better than an invalid selector`,
	}

	user := `Analyse this data broker privacy/opt-out page screenshot.
Return a JSON object — ONLY the JSON, no other text:
{
  "first_name": "<CSS selector for first name field, or empty string if not present>",
  "last_name":  "<CSS selector for last name field, or empty string if not present>",
  "email":      "<CSS selector for email field, e.g. input[type='email'] or input[name='email']>",
  "state":      "<CSS selector for state dropdown/input, or empty string if not present>",
  "submit":     "<CSS selector for submit button, e.g. button[type='submit'] or button — NO :contains()>",
  "has_captcha": <true if CAPTCHA is visible, false otherwise>,
  "notes":      "<one-sentence description of what you found>"
}`

	msg, err := v.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens: 512,
		System:    []anthropic.TextBlockParam{system},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(user),
				anthropic.NewImageBlockBase64(mime, imgB64),
			),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("haiku vision (find form): %w", err)
	}

	raw := strings.TrimSpace(extractText(msg))
	// Strip accidental markdown fences
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var sel FormSelectors
	if err := json.Unmarshal([]byte(raw), &sel); err != nil {
		return nil, fmt.Errorf("parse haiku response %q: %w", raw, err)
	}
	if sel.Notes == "NO_FORM_FOUND" {
		return nil, nil
	}
	return &sel, nil
}

// domTextFailureSignals are strong, low-ambiguity failure indicators that
// don't require visual judgment to trust. Deliberately narrow and literal —
// see the asymmetry note on checkDomTextFailure below for why this list
// stays conservative rather than trying to also catch success signals.
var domTextFailureSignals = []string{
	"invalid email",
	// "captcha" is a broad substring match - it would also match a page that
	// merely mentions CAPTCHA in unrelated copy (e.g. a footer note about bot
	// protection), not just an active challenge. That's an accepted tradeoff:
	// per the asymmetry above, over-triggering a "failure" is the safe
	// direction to be wrong in, since it just costs a manual re-check rather
	// than a false success.
	"captcha",
}

// checkDomTextFailure looks for an unambiguous failure signal in domText,
// skipping the Haiku vision call entirely when one is found.
//
// This is deliberately one-directional: it only ever returns a *failure*
// determination, never a success one. The asymmetry is intentional — a
// false "failed" costs the user a manual re-check, which is annoying but
// safe. A false "success" would mean this tool reports a privacy removal
// that never actually happened, which is a much worse failure mode for a
// tool whose entire premise is trustworthy privacy removal. So potential
// successes always still go through real vision validation; only the
// clear-failure cases get the free, deterministic fast path.
func checkDomTextFailure(domText string) (matched bool, message string) {
	lower := strings.ToLower(domText)
	for _, signal := range domTextFailureSignals {
		if strings.Contains(lower, signal) {
			return true, fmt.Sprintf("dom-text fast path: matched failure signal %q (no API call made)", signal)
		}
	}
	return false, ""
}

// ValidateOutcome determines whether an opt-out request was accepted.
// Returns (success, human-readable summary, error).
//
// domText is optional: pass up to 3000 chars of page innerText for additional
// signal. Pass "" to rely on screenshot only.
//
// Checks domText for a clear failure signal first (free, no API call) before
// falling back to Claude Haiku vision — see checkDomTextFailure for why this
// only short-circuits on failure, never on success.
func (v *Validator) ValidateOutcome(ctx context.Context, screenshotPNG []byte, siteName, domText string) (bool, string, error) {
	if matched, message := checkDomTextFailure(domText); matched {
		return false, message, nil
	}


	mime   := imgMIME(screenshotPNG)
	imgB64 := base64.StdEncoding.EncodeToString(screenshotPNG)

	system := anthropic.TextBlockParam{
		Text: `You audit opt-out form submissions on data broker websites.
Analyse a post-submission screenshot (and optional page text) to determine if the removal request succeeded.
Respond with JSON only. No markdown.

IMPORTANT — silent-redirect pattern: many data brokers redirect the user back to the
main privacy/data center page after a successful submission and send a confirmation email
instead of showing an on-page message. If the post-submission page looks like a normal
privacy center WITHOUT an error message (no "invalid email", no "please try again",
no CAPTCHA error), treat this as needs_email_confirmation=true and success=true.
Only mark success=false if you see an explicit error or the form is still empty/unfilled.`,
	}

	domSnippet := ""
	if domText != "" {
		domSnippet = fmt.Sprintf("\n\nPage text (first 2000 chars):\n%s", domText)
	}

	user := fmt.Sprintf(`This screenshot was taken immediately after submitting a data-removal request on %s.%s

Return JSON — ONLY the JSON:
{
  "success": <true if request was accepted, check-email shown, or normal privacy page with no error>,
  "message": "<what confirmation text or state you see>",
  "needs_email_confirmation": <true if they asked to check email OR if page looks like normal redirect with no error>,
  "needs_manual_action": <true if CAPTCHA, explicit error, or something clearly wrong>
}

SUCCESS signals: "request submitted", "check your email", "we received your request",
"your information will be removed", confirmation/thank-you page, OR main privacy
center page with no error message (silent redirect — confirmation sent by email).
FAILURE signals: explicit error message, "invalid email", form still visible and EMPTY,
CAPTCHA puzzle that needs solving.`, siteName, domSnippet)

	msg, err := v.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens: 256,
		System:    []anthropic.TextBlockParam{system},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(user),
				anthropic.NewImageBlockBase64(mime, imgB64),
			),
		},
	})
	if err != nil {
		return false, "", fmt.Errorf("haiku vision (validate outcome): %w", err)
	}

	raw := strings.TrimSpace(extractText(msg))
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var result struct {
		Success           bool   `json:"success"`
		Message           string `json:"message"`
		NeedsEmailConfirm bool   `json:"needs_email_confirmation"`
		NeedsManualAction bool   `json:"needs_manual_action"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return false, raw, fmt.Errorf("parse haiku outcome %q: %w", raw, err)
	}

	summary := result.Message
	if result.NeedsEmailConfirm {
		summary += " [EMAIL_CONFIRMATION_REQUIRED]"
	}
	if result.NeedsManualAction {
		summary += " [MANUAL_ACTION_REQUIRED]"
	}

	// Email confirmation = success (we submitted; they'll send a link)
	return result.Success || result.NeedsEmailConfirm, summary, nil
}

func extractText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		if txt := block.AsText(); txt.Text != "" {
			return txt.Text
		}
	}
	return ""
}
