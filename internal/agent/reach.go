package agent

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Reachability is what two independent checks concluded about a domain.
type Reachability struct {
	Domain    string
	Resolves  bool
	DNSErr    string
	Loaded    bool
	Title     string
	NavErr    string
	Parked    bool
	BodyChars int
}

// parkedSignals are phrases that appear on domain-parking and for-sale pages.
// A resolving domain that serves one of these is gone in every sense that
// matters here: there is no opt-out mechanism behind it and never will be.
var parkedSignals = []string{
	"this domain is for sale",
	"buy this domain",
	"domain may be for sale",
	"the domain has expired",
	"this domain has expired",
	"renew now to keep",
	"parked free, courtesy of",
	"godaddy.com/domainsearch",
	"related searches",
}

// CheckReachability asks two independent questions about a domain: does it
// resolve, and does a real browser get a page from it.
//
// Both are required before anything is concluded. DNS alone is not enough - a
// resolver hiccup, a captive network, or a temporarily misconfigured zone all
// look identical to a domain that is genuinely gone, and this project has
// already spent days on classifications made from exactly one signal that
// happened to be ambiguous. A domain that neither resolves nor loads has
// failed two unrelated checks, which is a different kind of claim.
func CheckReachability(ctx context.Context, domain string) *Reachability {
	r := &Reachability{Domain: domain}

	resolveCtx, cancelDNS := context.WithTimeout(ctx, 10*time.Second)
	defer cancelDNS()
	addrs, err := net.DefaultResolver.LookupHost(resolveCtx, domain)
	if err != nil {
		r.DNSErr = err.Error()
	} else {
		r.Resolves = len(addrs) > 0
	}

	taskCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()
	taskCtx, cancelTimeout := context.WithTimeout(taskCtx, 35*time.Second)
	defer cancelTimeout()

	var page struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	navErr := chromedp.Run(taskCtx,
		chromedp.Navigate("https://"+domain+"/"),
		chromedp.Sleep(4*time.Second),
		chromedp.Evaluate(`({title: document.title || '', text: (document.body ? document.body.innerText : '').slice(0,1500)})`, &page),
	)
	if navErr != nil {
		r.NavErr = navErr.Error()
		return r
	}

	r.Loaded = true
	r.Title = page.Title
	r.BodyChars = len(strings.TrimSpace(page.Text))

	hay := strings.ToLower(page.Title + " " + page.Text)
	for _, sig := range parkedSignals {
		if strings.Contains(hay, sig) {
			r.Parked = true
			break
		}
	}
	return r
}

// Verdict reports whether the domain is dead, and why. An empty string means
// the domain is alive enough that deadness is not the explanation for anything.
func (r *Reachability) Verdict() (dead bool, reason string) {
	switch {
	case !r.Resolves && !r.Loaded:
		return true, fmt.Sprintf("does not resolve and does not load (dns: %s)", firstLine(r.DNSErr))
	case r.Parked:
		return true, fmt.Sprintf("resolves, but serves a parked/for-sale page (%q)", r.Title)
	case r.Loaded && r.BodyChars < 40:
		// Not called dead: an empty body is equally consistent with a
		// client-rendered app that needed longer than the wait above.
		return false, fmt.Sprintf("loads but rendered almost nothing (%d chars) - look before concluding", r.BodyChars)
	case !r.Resolves && r.Loaded:
		return false, "did not resolve here but the browser loaded it anyway - treat the DNS failure as local noise"
	}
	return false, ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	if s == "" {
		return "ok"
	}
	return s
}
