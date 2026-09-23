package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
)

// ProbeInput describes one form control as chromedp actually sees it.
type ProbeInput struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Type        string `json:"type"`
	Placeholder string `json:"placeholder"`
	Visible     bool   `json:"visible"`
	// ClickableLabel is set when the control is zero-size (hidden) but sits
	// inside or is targeted by a visible <label>. That label is what a click
	// must target - chromedp.Click waits for its own target to become
	// visible, and a hidden input never does, so clicking the input directly
	// hangs until the context deadline. This exact pattern cost four days on
	// CheckPeople, where the timeout got misread as a bot defense.
	ClickableLabel string `json:"clickableLabel"`
}

// ProbeResult is what a headless chromedp session observes at a URL.
type ProbeResult struct {
	URL         string       `json:"url"`
	Title       string       `json:"title"`
	Challenge   bool         `json:"challenge"`
	Captcha     bool         `json:"captcha"`
	CaptchaKind string       `json:"captchaKind"`
	Inputs      []ProbeInput `json:"inputs"`
	BodySnippet string       `json:"bodySnippet"`
	NavErr      string       `json:"-"`
}

const probeJS = `(function(){
  var t = document.title || '';
  var bodyText = document.body ? document.body.innerText : '';
  var challengeRe = /just a moment|attention required|verifying you are human|checking your browser|ddos protection|enable javascript and cookies/i;
  var cap = document.querySelector('.g-recaptcha, iframe[src*="recaptcha"], iframe[src*="turnstile"], iframe[src*="hcaptcha"], [class*="turnstile"]');
  var inputs = Array.prototype.slice.call(document.querySelectorAll('input, textarea, select'), 0, 30).map(function(e){
    var r = e.getBoundingClientRect();
    var labelSel = '';
    if (r.width === 0 || r.height === 0) {
      var p = e.closest ? e.closest('label') : null;
      if (p) {
        labelSel = p.className ? 'label.' + String(p.className).trim().split(/\s+/).join('.') : 'label';
      } else if (e.id) {
        if (document.querySelector('label[for="' + e.id + '"]')) {
          labelSel = 'label[for="' + e.id + '"]';
        }
      }
    }
    return {
      name: e.name || '', id: e.id || '',
      type: e.type || e.tagName.toLowerCase(),
      placeholder: e.placeholder || '',
      visible: r.width > 0 && r.height > 0,
      clickableLabel: labelSel
    };
  });
  return {
    url: location.href, title: t,
    challenge: challengeRe.test(t + ' ' + bodyText.slice(0, 500)),
    captcha: !!cap,
    captchaKind: cap ? String(cap.outerHTML).slice(0, 70) : '',
    inputs: inputs,
    bodySnippet: bodyText.slice(0, 300)
  };
})()`

// Probe navigates to url with a real headless browser and reports what that
// browser actually receives - nothing is filled and nothing is submitted.
//
// It exists because the recurring, expensive mistake on this project has been
// classifying a site from indirect evidence: a plain HTTP fetch returning 403
// (which says nothing about a real browser), a handler timing out (which was a
// hidden checkbox, not a defense), or a page loading fine in the operator's own
// Chrome (which says nothing about what chromedp gets - Spokeo serves a human
// the form and chromedp a 403). Every one of those produced a wrong
// blocker_type that survived for days. Looking with the same tool that does the
// work is the only evidence that settles it.
//
// Deliberately does NOT wait for any specific selector: a page that never
// renders the expected form is precisely the signal worth capturing, and
// WaitVisible would turn that into an opaque timeout instead.
func Probe(ctx context.Context, url string) (*ProbeResult, error) {
	taskCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 45*time.Second)
	defer cancelTimeout()

	var res ProbeResult
	err := chromedp.Run(taskCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(6*time.Second), // let client-rendered forms and any interstitial settle
		chromedp.Evaluate(probeJS, &res),
	)
	if err != nil {
		return &res, fmt.Errorf("probe %s: %w", url, err)
	}
	return &res, nil
}

// SuggestedBlocker maps a probe result to the blocker_type it implies, or ""
// when the page looks workable. Advisory only - the operator still decides.
func (r *ProbeResult) SuggestedBlocker() string {
	// A navigation error means the browser never completed the request - it
	// could not start, DNS failed, the context expired. None of that is
	// evidence about the site, and treating it as such is the exact mistake
	// this whole file exists to stop. It once reported bot_defended for a
	// local Chrome launch failure. No response, no conclusion.
	if r.NavErr != "" {
		return ""
	}
	switch {
	case r.Challenge:
		return "bot_defended"
	case r.Captcha:
		return "bot_defended"
	case r.Title == "403 Forbidden" || r.Title == "Access Denied":
		// A title at all means the site answered us. That is evidence.
		return "bot_defended"
	case len(r.Inputs) == 0:
		return "" // nothing rendered; could be no_mechanism, could be the wrong URL - operator's call
	}
	return ""
}
