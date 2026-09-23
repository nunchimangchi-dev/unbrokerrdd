package agent

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// PrivacyContact is a candidate address for a CCPA/CPRA deletion request,
// together with where it was found so a human can check it.
type PrivacyContact struct {
	Email  string `json:"email"`
	Source string `json:"source"` // the page it was found on
	Rank   int    `json:"rank"`   // lower is better
	Why    string `json:"why"`
}

// DiscoverResult is everything one site's discovery pass turned up.
type DiscoverResult struct {
	Domain     string
	PolicyURL  string
	Candidates []PrivacyContact
	Notes      string
}

// privacyLinkJS finds links that look like they lead to a privacy policy, a
// CCPA notice, or a do-not-sell page, preferring the most specific.
const privacyLinkJS = `(function(){
  var want = [
    {re: /do not sell|do-not-sell|dont sell/i, w: 0},
    {re: /ccpa|cpra|california privacy/i,      w: 1},
    {re: /privacy (policy|notice)/i,           w: 2},
    {re: /privacy/i,                           w: 3},
    {re: /opt.?out|data request|your rights/i, w: 4}
  ];
  var out = [];
  Array.prototype.slice.call(document.querySelectorAll('a[href]')).forEach(function(a){
    var hay = (a.innerText || '') + ' ' + (a.getAttribute('href') || '');
    for (var i = 0; i < want.length; i++) {
      if (want[i].re.test(hay)) { out.push({href: a.href, w: want[i].w, text: (a.innerText||'').trim().slice(0,60)}); break; }
    }
  });
  out.sort(function(x,y){ return x.w - y.w; });
  return out.slice(0, 5);
})()`

// pageEmailsJS pulls addresses from mailto: links and from the rendered text.
// pageEmailsJS pulls addresses from mailto: links and from the document text.
//
// It reads textContent, not innerText. Privacy policies are routinely served
// under a cookie-consent overlay, inside collapsed accordions, or in sections
// rendered off-screen - all of which innerText omits because they are not
// visible. The first pass of this used innerText and reported "no address
// published" for sites whose policy page states one plainly; the text was
// there, just not painted. Consent banners are read around, never clicked.
const pageEmailsJS = `(function(){
  var found = [];
  function scan(doc){
    try {
      Array.prototype.slice.call(doc.querySelectorAll('a[href^="mailto:"]')).forEach(function(a){
        found.push(a.getAttribute('href').replace(/^mailto:/i,'').split('?')[0]);
      });
    } catch(e) {}
  }
  scan(document);
  var text = '';
  var el = document.body || document.documentElement;
  if (el) { text = el.textContent || ''; }
  // Policies are often served inside a same-origin iframe by a consent or
  // legal-hosting widget. Cross-origin frames throw and are skipped.
  Array.prototype.slice.call(document.querySelectorAll('iframe')).forEach(function(f){
    try {
      var d = f.contentDocument;
      if (d && d.body) { scan(d); text += '\n' + (d.body.textContent || ''); }
    } catch(e) {}
  });
  return {mailto: found, text: text.slice(0, 60000)};
})()`

var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// rankEmail scores an address by how likely it is to be the right destination
// for a statutory deletion request. A privacy mailbox is a far better target
// than a general support queue; a webmaster or press address is nearly useless.
func rankEmail(addr string) (int, string) {
	local := strings.ToLower(addr)
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	switch {
	case strings.Contains(local, "privacy"):
		return 0, "privacy mailbox"
	case strings.Contains(local, "ccpa"), strings.Contains(local, "gdpr"), strings.Contains(local, "dpo"):
		return 1, "data-protection mailbox"
	case strings.Contains(local, "legal"), strings.Contains(local, "compliance"):
		return 2, "legal mailbox"
	case strings.Contains(local, "optout"), strings.Contains(local, "opt-out"), strings.Contains(local, "removal"):
		return 2, "opt-out mailbox"
	case strings.Contains(local, "support"), strings.Contains(local, "help"), strings.Contains(local, "contact"):
		return 4, "general support - may not be handled as a rights request"
	case strings.Contains(local, "info"), strings.Contains(local, "admin"), strings.Contains(local, "hello"):
		return 5, "generic mailbox"
	case strings.Contains(local, "press"), strings.Contains(local, "sales"), strings.Contains(local, "webmaster"),
		strings.Contains(local, "noreply"), strings.Contains(local, "no-reply"):
		return 9, "wrong department"
	}
	return 6, "unclassified"
}

// DiscoverPrivacyContact looks for an address a deletion request can be sent
// to, by following the site's own privacy links rather than guessing URLs.
//
// Guessing paths (/privacy, /privacy-policy, ...) is the same mistake as
// guessing a search URL: a 404 is indistinguishable from "no policy page", and
// concluding "no mechanism" from a guessed 404 would be another
// classification made from evidence that was never about the site. Following
// the site's own links means a miss is a real miss.
//
// Read-only. Nothing is sent, and no address is contacted.
func DiscoverPrivacyContact(ctx context.Context, domain string) (*DiscoverResult, error) {
	res := &DiscoverResult{Domain: domain}

	taskCtx, cancel := NewBrowserContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 150*time.Second)
	defer cancelTimeout()

	var links []struct {
		Href string `json:"href"`
		W    int    `json:"w"`
		Text string `json:"text"`
	}
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate("https://"+domain+"/"),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(privacyLinkJS, &links),
	); err != nil {
		return res, fmt.Errorf("load %s: %w", domain, err)
	}

	seen := map[string]bool{}
	collect := func(sourceURL string) error {
		var page struct {
			Mailto []string `json:"mailto"`
			Text   string   `json:"text"`
		}
		if err := chromedp.Run(taskCtx, chromedp.Navigate(sourceURL)); err != nil {
			return err
		}
		// Poll until the text stops growing rather than sleeping a fixed
		// interval. A single-page app can serve a 1kB shell for several
		// seconds before the policy itself arrives, and reading during that
		// window reports "this site publishes no address" about a page that
		// had not loaded yet - a no_mechanism claim made from a race.
		prev := -1
		for i := 0; i < 12; i++ {
			if err := chromedp.Run(taskCtx,
				chromedp.Sleep(1500*time.Millisecond),
				chromedp.Evaluate(pageEmailsJS, &page),
			); err != nil {
				return err
			}
			if len(page.Mailto) > 0 || (len(page.Text) == prev && len(page.Text) > 500) {
				break
			}
			prev = len(page.Text)
		}
		if os.Getenv("DISCOVER_DEBUG") != "" {
			fmt.Printf("      [debug] %s: %d chars of text, %d mailto links\n", sourceURL, len(page.Text), len(page.Mailto))
			shown := 0
			for i, ch := range page.Text {
				if ch == '@' && shown < 6 {
					shown++
					lo, hi := i-40, i+40
					if lo < 0 {
						lo = 0
					}
					if hi > len(page.Text) {
						hi = len(page.Text)
					}
					fmt.Printf("      [debug]   @ context: %q\n", strings.ReplaceAll(page.Text[lo:hi], "\n", " "))
				}
			}
		}
		addrs := append([]string{}, page.Mailto...)
		addrs = append(addrs, emailRe.FindAllString(page.Text, 60)...)
		for _, a := range addrs {
			a = strings.TrimSpace(strings.Trim(a, ".,;:()<>\"'"))
			key := strings.ToLower(a)
			if a == "" || seen[key] {
				continue
			}
			// Images and asset filenames regularly match the address pattern.
			if strings.HasSuffix(key, ".png") || strings.HasSuffix(key, ".jpg") ||
				strings.HasSuffix(key, ".gif") || strings.HasSuffix(key, ".webp") {
				continue
			}
			seen[key] = true
			rank, why := rankEmail(a)
			res.Candidates = append(res.Candidates, PrivacyContact{Email: a, Source: sourceURL, Rank: rank, Why: why})
		}
		return nil
	}

	if len(links) == 0 {
		res.Notes = "no privacy/CCPA link found on the homepage"
	} else {
		res.PolicyURL = links[0].Href
		if err := collect(links[0].Href); err != nil {
			res.Notes = fmt.Sprintf("privacy page did not load: %v", err)
		}
	}

	// Fall back to the homepage only if the policy page yielded nothing;
	// homepage addresses are usually sales or support, hence the lower rank.
	if len(res.Candidates) == 0 {
		if err := collect("https://" + domain + "/"); err != nil && res.Notes == "" {
			res.Notes = fmt.Sprintf("homepage did not load: %v", err)
		}
	}

	sort.SliceStable(res.Candidates, func(i, j int) bool {
		return res.Candidates[i].Rank < res.Candidates[j].Rank
	})
	return res, nil
}

// Best returns the highest-ranked candidate, or nil when nothing usable was
// found. Addresses ranked as the wrong department are never returned as best -
// a deletion request to a sales inbox is not a request, it is noise.
func (d *DiscoverResult) Best() *PrivacyContact {
	if len(d.Candidates) == 0 || d.Candidates[0].Rank >= 9 {
		return nil
	}
	return &d.Candidates[0]
}
