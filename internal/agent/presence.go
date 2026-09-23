package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/chromedp/chromedp"
)

// Finding is what a presence check concluded about a single site.
type Finding string

const (
	// FindingPresent means the subject appears on the site. Removal work is warranted.
	FindingPresent Finding = "present"
	// FindingAbsent means the search ran cleanly and the subject is not there.
	// Nothing to remove - this is a real, earned outcome, not a skipped chore.
	FindingAbsent Finding = "absent"
	// FindingUndetermined means the check could not be trusted either way:
	// the page challenged us, errored, returned nothing readable, or the model
	// declined to call it. A human has to look.
	FindingUndetermined Finding = "undetermined"
)

// PresenceResult is the outcome of one read-only presence check.
type PresenceResult struct {
	SearchURL string  `json:"-"` // holds the subject's name; never printed
	Finding   Finding `json:"finding"`
	Evidence  string  `json:"evidence"`
	Challenge bool    `json:"challenge"`
	PageChars int     `json:"pageChars"`
}

// presenceTextJS pulls the rendered text plus the same challenge signals probe
// looks for. Presence checks and probes ask different questions of the same
// page, so the challenge detection stays in one place - see challengeRe.
const presenceTextJS = `(function(){
  var t = document.title || '';
  var body = document.body ? document.body.innerText : '';
  var challengeRe = /just a moment|attention required|verifying you are human|checking your browser|ddos protection|enable javascript and cookies/i;
  return {
    title: t,
    text: body.slice(0, 6000),
    challenge: challengeRe.test(t + ' ' + body.slice(0, 500)),
    blocked: /403 forbidden|access denied/i.test(t)
  };
})()`

// BuildSearchURL substitutes a subject's name into a site's search template.
// Supported placeholders: {name} {first} {last} {first+last} (plus-joined).
// Values are query-escaped, so a name with spaces or punctuation is safe.
func BuildSearchURL(template, fullName string) (string, error) {
	parts := strings.Fields(fullName)
	if len(parts) < 2 {
		return "", fmt.Errorf("subject name %q does not have a first and last name", redactName(fullName))
	}
	first, last := parts[0], parts[len(parts)-1]

	r := strings.NewReplacer(
		"{name}", url.QueryEscape(fullName),
		"{first}", url.QueryEscape(first),
		"{last}", url.QueryEscape(last),
		"{first+last}", url.QueryEscape(first+"+"+last),
	)
	out := r.Replace(template)
	if strings.Contains(out, "{") {
		return "", fmt.Errorf("search template has an unsubstituted placeholder: %s", template)
	}
	return out, nil
}

// CheckPresence loads a site's search results for the subject and decides
// whether the subject appears there. It navigates and reads - it never fills a
// form, never submits anything, and never writes to the attempts log.
//
// Why this exists: most of this project's remaining targets have never been
// attempted, and for a large share of them the honest answer is not "removal
// failed" but "there was never a record to remove". Until that question is
// asked, the denominator in any claim about coverage is a list of sites the
// subject may not even be on. Answering it is cheap, read-only, and is the one
// determination the automation can still earn unattended.
func CheckPresence(ctx context.Context, apiKey, siteName, searchURL, subjectName string) (*PresenceResult, error) {
	taskCtx, cancel := NewBrowserContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 60*time.Second)
	defer cancelTimeout()

	var page struct {
		Title     string `json:"title"`
		Text      string `json:"text"`
		Challenge bool   `json:"challenge"`
		Blocked   bool   `json:"blocked"`
	}

	res := &PresenceResult{SearchURL: searchURL}

	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(searchURL),
		chromedp.Sleep(6*time.Second), // results are usually client-rendered
		chromedp.Evaluate(presenceTextJS, &page),
	); err != nil {
		res.Finding = FindingUndetermined
		res.Evidence = fmt.Sprintf("navigation failed: %v", err)
		return res, nil
	}

	res.Challenge = page.Challenge
	res.PageChars = len(page.Text)

	// Deterministic short-circuits run in one direction only - see the note on
	// the asymmetry below. Each of these ends in "a human must look", never in
	// a clean bill of health.
	switch {
	case page.Challenge:
		res.Finding = FindingUndetermined
		res.Evidence = "bot challenge interstitial - the search never ran"
		return res, nil
	case page.Blocked:
		res.Finding = FindingUndetermined
		res.Evidence = fmt.Sprintf("site refused the browser: %q", page.Title)
		return res, nil
	case strings.TrimSpace(page.Text) == "":
		res.Finding = FindingUndetermined
		res.Evidence = "page rendered no readable text"
		return res, nil
	}

	if apiKey == "" {
		res.Finding = FindingUndetermined
		res.Evidence = "no ANTHROPIC_API_KEY; page loaded but nothing classified it"
		return res, nil
	}

	finding, evidence, err := classifyPresence(ctx, apiKey, siteName, subjectName, page.Text)
	if err != nil {
		res.Finding = FindingUndetermined
		res.Evidence = fmt.Sprintf("classification failed: %v", err)
		return res, nil
	}
	res.Finding, res.Evidence = finding, ScrubSubject(evidence, subjectName)
	return res, nil
}

// classifyPresence asks Haiku whether the subject appears in the search results.
//
// The asymmetry here is the mirror image of checkDomTextFailure's, and for the
// same reason. There, the dangerous claim was a success that never happened;
// here it is "absent" - telling someone they are not listed on a data broker
// when they are. That is the exact false reassurance this whole project exists
// to correct, so "absent" is the only finding that has to be argued for. Every
// uncertain path above returns undetermined instead, which costs a manual look
// and nothing else.
func classifyPresence(ctx context.Context, apiKey, siteName, subjectName, pageText string) (Finding, string, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	system := anthropic.TextBlockParam{
		Text: `You read search-results pages from people-search and directory websites and
decide whether a specific person appears in the results.

Respond with JSON only. No markdown fences, no commentary.

Decide between exactly three answers:
  "present"      - a result matching the person's name is listed
  "absent"       - the search ran and returned no result matching that name
                   (an explicit "no results" message, or results that are
                   clearly other people)
  "undetermined" - you cannot tell: the page is a search form rather than
                   results, requires a login or payment to show names, shows an
                   error, or the text is truncated mid-results

Answer "absent" ONLY when the page genuinely shows a completed search with no
match. A wrong "absent" tells someone they are not listed when they are, which
is worse than asking them to look themselves.

These are all "undetermined", never "absent":
  - the page is a homepage, landing page, or a browsable A-Z / category index
    rather than the result of the search that was requested
  - the page shows an empty search form and no results section
  - the page lists people, but there is no sign the list was filtered by the
    requested name (e.g. it reads as a generic directory)
  - the page says results exist but requires payment or sign-in to see names

A real "absent" page almost always echoes the searched name back - "no results
for X", "we could not find X", "0 results". If nothing on the page indicates
that THIS name was searched for, you cannot conclude the person is not there.

Match names generously: middle names, initials, suffixes, and different
capitalisation still count as the same person. Do not require an exact string.`,
	}

	user := fmt.Sprintf(`Site: %s
Person being searched for: %s

Search results page text:
---
%s
---

Return JSON only:
{
  "finding": "present" | "absent" | "undetermined",
  "evidence": "<one short sentence quoting or describing what decided it>"
}`, siteName, subjectName, pageText)

	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeHaiku4_5_20251001,
		MaxTokens: 256,
		System:    []anthropic.TextBlockParam{system},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})
	if err != nil {
		return FindingUndetermined, "", fmt.Errorf("haiku presence: %w", err)
	}

	raw := strings.TrimSpace(extractText(msg))
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var out struct {
		Finding  string `json:"finding"`
		Evidence string `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return FindingUndetermined, "", fmt.Errorf("parse haiku presence %q: %w", raw, err)
	}

	switch Finding(out.Finding) {
	case FindingPresent, FindingAbsent:
		return Finding(out.Finding), out.Evidence, nil
	default:
		return FindingUndetermined, out.Evidence, nil
	}
}

// ScrubSubject removes the subject's name from text that came back from a
// page or a model.
//
// This is not cosmetic. Presence evidence is written to the brokers table, and
// that database is synced to the production host, where it is served behind
// admin endpoints. Broker rows have always been free of personal data - that
// property is what made the sync safe to reason about - and a classifier that
// helpfully quotes "no results for <real name>" would have silently ended it.
// Scrubbing happens here, at the boundary where model output enters the
// program, so neither the terminal nor the database can acquire the name by
// some later code path forgetting to ask.
func ScrubSubject(text, fullName string) string {
	out := text
	for _, part := range strings.Fields(fullName) {
		if len(part) < 2 {
			continue
		}
		masked := part[:1] + "***"
		// Case-insensitive replace without regex-escaping surprises in names.
		for {
			idx := strings.Index(strings.ToLower(out), strings.ToLower(part))
			if idx < 0 {
				break
			}
			out = out[:idx] + masked + out[idx+len(part):]
		}
	}
	return out
}

// redactName masks a subject name for terminal output and error strings.
// Presence checks put the real name in a URL and read it back in page text;
// none of that belongs in a log, a terminal scrollback, or a pasted report.
func redactName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p[:1] + "***"
	}
	return strings.Join(out, " ")
}

// RedactURL masks any query-string value in a search URL before display, since
// the interesting ones hold the subject's name.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	q := u.Query()
	for k := range q {
		q.Set(k, "***")
	}
	u.RawQuery = q.Encode()
	// Names also appear as path segments on some sites (/name/first-last).
	segs := strings.Split(u.Path, "/")
	for i, s := range segs {
		if strings.Contains(s, "-") && len(s) > 3 {
			segs[i] = "***"
		}
	}
	u.Path = strings.Join(segs, "/")
	return u.String()
}

// Default control and decoy names for template verification. The control must
// be common enough that any working people-search returns something; the decoy
// must be implausible enough that any working search returns nothing.
const (
	DefaultControlName = "John Smith"
	DefaultDecoyName   = "Zephrina Qualtrough"
)

// TemplateVerdict is the result of testing a candidate search template.
type TemplateVerdict struct {
	Control   *PresenceResult
	Decoy     *PresenceResult
	Trusted   bool
	Reasoning string
}

// VerifyTemplate decides whether a candidate search URL template can be
// trusted to answer the presence question, by running a two-sided control
// experiment against it: a name that must be found, and a name that cannot be.
//
// This exists because the first template ever tested here looked like it
// worked and did not. The site ignored the query parameter and served its
// browsable directory index instead; the classifier read a page full of other
// people's names, found no match, and returned "absent". Every part of that
// chain behaved correctly and the answer was still a false all-clear - which
// is the one answer this project must never produce.
//
// A single search cannot distinguish "you are not listed here" from "this URL
// does not search". Two can. If a name that is certainly present also comes
// back absent, the template is broken, whatever it did for any other name.
func VerifyTemplate(ctx context.Context, apiKey, siteName, template, controlName, decoyName string) (*TemplateVerdict, error) {
	if controlName == "" {
		controlName = DefaultControlName
	}
	if decoyName == "" {
		decoyName = DefaultDecoyName
	}

	run := func(name string) (*PresenceResult, error) {
		u, err := BuildSearchURL(template, name)
		if err != nil {
			return nil, err
		}
		return CheckPresence(ctx, apiKey, siteName, u, name)
	}

	control, err := run(controlName)
	if err != nil {
		return nil, fmt.Errorf("control run: %w", err)
	}
	decoy, err := run(decoyName)
	if err != nil {
		return nil, fmt.Errorf("decoy run: %w", err)
	}

	v := &TemplateVerdict{Control: control, Decoy: decoy}
	switch {
	case control.Finding == FindingPresent && decoy.Finding == FindingAbsent:
		v.Trusted = true
		v.Reasoning = "control found, decoy not found - the URL really searches, and its negatives mean something"
	case control.Finding == FindingAbsent:
		v.Reasoning = "control name came back absent; a working search would have found it - the template does not search, or the site needs more than a name"
	case strings.Contains(control.Evidence, "navigation failed") || strings.Contains(decoy.Evidence, "navigation failed"):
		v.Reasoning = "the browser never reached the page - this is about the URL or the domain, not about what the site would have shown"
	case control.Finding == FindingUndetermined || decoy.Finding == FindingUndetermined:
		v.Reasoning = "at least one run could not be read (challenge, login wall, or no results section) - nothing can be concluded from this URL"
	case decoy.Finding == FindingPresent:
		v.Reasoning = "a name that cannot exist was reported present - the page is not filtering by the query at all"
	default:
		v.Reasoning = "inconclusive combination"
	}
	return v, nil
}
