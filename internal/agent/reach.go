package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Reachability is what two independent checks concluded about a domain.
type Reachability struct {
	Domain   string
	Resolves bool
	// DNSDefinitive is true only when the resolver gave a real answer: the
	// name exists, or it authoritatively does not (NXDOMAIN). A timeout or a
	// temporary SERVFAIL is not an answer about the domain, it is the absence
	// of one, and must never be read as evidence that a site is gone.
	DNSDefinitive bool
	DNSErr        string
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

	r.Resolves, r.DNSDefinitive, r.DNSErr = resolve(ctx, domain)

	taskCtx, cancel := NewBrowserContext(ctx)
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
	// The DNS check and the browser check are not independent: the browser
	// resolves the same name through the same resolver, so one slow resolver
	// fails both and looks like corroboration. A sweep built on that called
	// businesssearch.sos.ca.gov - the California Secretary of State - a dead
	// domain, along with nine others, purely because the resolver was
	// saturated. Only an authoritative answer counts.
	case r.Parked:
		// Direct evidence: the page loaded and said so itself. Whatever DNS
		// did is irrelevant once a real response is in hand.
		return true, fmt.Sprintf("resolves, but serves a parked/for-sale page (%q)", r.Title)
	case !r.DNSDefinitive:
		return false, fmt.Sprintf("resolver never answered (%s) - no conclusion; re-run when the network is quiet", firstLine(r.DNSErr))
	case !r.Resolves && !r.Loaded:
		return true, fmt.Sprintf("the name does not exist and nothing loads (dns: %s)", firstLine(r.DNSErr))
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

// fallbackResolvers are consulted when the system resolver does not answer.
//
// The system resolver here is Tailscale MagicDNS (100.100.100.100). It is
// healthy - it answers google.com in 34ms - but for a domain whose
// authoritative nameservers no longer respond it hangs and times out rather
// than returning NXDOMAIN. Those are exactly the domains a reachability sweep
// asks about, and the browser fails identically because it shares the
// resolver. One sweep built on that called ten live domains dead, including
// businesssearch.sos.ca.gov, the California Secretary of State. A resolver
// that is correct for everything you spot-check and silent only for what you
// are actually asking about is worse than one that is plainly down. Asking a
// second, independent resolver is what makes the two checks genuinely
// independent rather than two views of one failure.
var fallbackResolvers = []string{"1.1.1.1:53", "8.8.8.8:53"}

func resolverFor(server string) *net.Resolver {
	if server == "" {
		return net.DefaultResolver
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 8 * time.Second}
			return d.DialContext(ctx, network, server)
		},
	}
}

// resolve looks the domain up, distinguishing a real answer from a
// non-answer, and escalates past a resolver that is not answering.
//
// Returns (exists, definitive, errText). definitive is true only when some
// resolver gave a real answer - the name resolves, or authoritatively does
// not. Timeouts and SERVFAILs are non-answers: they are retried, then tried
// against the fallback resolvers, and only reported as inconclusive if every
// one of them declines to answer. A non-answer is never evidence of absence.
func resolve(ctx context.Context, domain string) (exists, definitive bool, errText string) {
	servers := append([]string{""}, fallbackResolvers...)
	var last error

	for _, server := range servers {
		r := resolverFor(server)
		for attempt := 0; attempt < 2; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return false, false, "lookup cancelled"
				case <-time.After(2 * time.Second):
				}
			}
			lookupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			addrs, err := r.LookupHost(lookupCtx, domain)
			cancel()

			if err == nil {
				return len(addrs) > 0, true, ""
			}
			last = err

			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) {
				if dnsErr.IsNotFound {
					// Authoritative: no address to reach. This covers both a
					// name that does not exist and a zone that publishes no
					// address records - operationally the same thing here,
					// since either way there is no host to send a request to.
					return false, true, fmt.Sprintf("no address records for %s (answered by %s)", domain, resolverLabel(server))
				}
				if dnsErr.IsTimeout || dnsErr.IsTemporary {
					continue // no answer yet from this resolver
				}
			}
			break // unrecognised error - move to the next resolver
		}
	}

	if last == nil {
		return false, false, "no answer from any resolver"
	}
	return false, false, "no resolver answered: " + last.Error()
}

// resolverLabel names which resolver actually produced an answer. Go's own
// DNSError text names the resolver from the system configuration even when the
// lookup went to a custom one, which made a fallback answer read as though the
// broken system resolver had confirmed it.
func resolverLabel(server string) string {
	if server == "" {
		return "the system resolver"
	}
	return server
}
