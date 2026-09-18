// Package agent defines the Agent interface and shared security primitives.
package agent

import (
	"fmt"
	"net/url"
	"strings"
)

// AllowedDomains is the explicit set of domains the browser agent may navigate to.
// Every broker URL and every opt-out URL must match before the browser opens.
// Add a domain here only when it appears in databrokergo.md.
//
// Security contract: ValidateURL is called before any chromedp.Navigate call.
// If it returns an error, the navigation is aborted and the broker is marked failed.
var AllowedDomains = map[string]bool{
	// ── Opt-out infrastructure ─────────────────────────────────────────
	// truthfinder.com/privacy-center's "Delete My User Data" button is
	// account/visitor data deletion ONLY — TruthFinder's own page states
	// deleting it does NOT suppress your public Background Report. Verified
	// live 2026-09-17; Strategy 1 was wrongly targeting this. Kept in the
	// allowlist as a dead entry (nothing navigates here anymore) rather than
	// silently removed, so the mistake stays visible in history.
	"truthfinder.com": true,
	// The real suppression flow: PeopleConnect's shared suppression portal,
	// which per TruthFinder's own privacy center covers TruthFinder,
	// Intelius, USSearch, Instant Checkmate, PeopleFinder, PeopleLookup,
	// Classmates, Spock, and Zabasearch under one request.
	"suppression.peopleconnect.us": true,
	"lookup.icann.org":             true, // Strategy 5: WHOIS lookups

	// ── Strategy 1: TruthFinder affiliates ────────────────────────────
	"backgroundcheckme.org":    true,
	"newyorkpublicrecords.org": true,
	"oregonpublicrecords.org":  true,
	"publicrecordscenter.org":  true,
	"publicrecordsreviews.com": true,
	"publicsrecords.com":       true,
	"top4backgroundchecks.com": true,

	// ── Strategy 2: Hidden opt-out pages ──────────────────────────────
	"checkpeople.com":              true, // verified live 2026-09-17; replaces unverified peeplookup.com
	"advancedbackgroundchecks.com": true, // verified live 2026-09-18 — see strategy2_advancedbackgroundchecks.go
	"ohioresidentdirectory.com":    true,
	"peoplesearchexpert.com":       true,
	"peoplefastfind.com":           true,
	"backgroundchecks.org":         true,
	"backgroundchecks.me":          true,
	"freepeople-search.com":        true,
	"usawhitepages.com":            true,
	"usaphonesbook.com":            true,
	"locatefamily.com":             true,
	"reunion.com":                  true,
	"addrhistory.com":              true,
	"alumnius.net":                 true,
	"jailbase.com":                 true,
	// Added 2026-09-18: the 8 BADBOOL sites brought into the registry
	// today, plus Spokeo's real handler (strategy2_spokeo.go).
	"spokeo.com":                true,
	"beenverified.com":          true,
	"smartbackgroundchecks.com": true,
	"nuwber.com":                true,
	"clustal.org":               true,
	"thatsthem.com":             true,
	"familytreenow.com":         true,
	"usphonebook.com":           true,
	"radaris.com":               true,

	// ── Strategy 3: Privacy page discovery ────────────────────────────
	"alignable.com":                  true,
	"allpeople.biz":                  true,
	"enpnetwork.com":                 true,
	"konaequity.com":                 true,
	"morningstar.com":                true,
	"listmatch.com":                  true,
	"cityzor.com":                    true,
	"criminalpages.com":              true,
	"genealogic.review":              true,
	"lacountyarrestrecords.com":      true,
	"myfunnyprofile.com":             true,
	"northdakotapeoplerecords.com":   true,
	"massachusettspeoplerecords.com": true,
	"old-friends.co":                 true,
	"oldphonebook.com":               true,
	"opendatausa.com":                true,
	"stageresearch.com":              true,
	"realtyverify.com":               true,
	"parcellookup.com":               true,
	"census-info.us":                 true,
	"knsee.com":                      true,
	"pininthemap.com":                true,
	"pplchecker.com":                 true,
	"h1bdata.info":                   true,
	"image-maps.com":                 true,

	// ── Strategy 4: Business directories ─────────────────────────────
	"amfibi.com":                   true,
	"azcc.gov":                     true,
	"ausibiz.com":                  true,
	"businesssearch.sos.ca.gov":    true,
	"corporationscanada.ic.gc.ca":  true,
	"ecorp.sos.ga.gov":             true,
	"merchantcircle.com":           true,
	"misterwhat.com":               true,
	"opencorporates.com":           true,
	"panjiva.com":                  true,
	"stateinformationservices.com": true,
	"yellowpages.com":              true,
	"ccfs.sos.wa.gov":              true,
	"wealthminder.com":             true,
	"websiteoutlook.com":           true,
	"zaubee.com":                   true,
	"localchiros.com":              true,
	"ppp-loan.info":                true,

	// ── Strategy 5: Phone directory WHOIS ────────────────────────────
	"1called.com":            true,
	"1who.net":               true,
	"411reverselookup.ca":    true,
	"calleridtest.com":       true,
	"phonecheckpro.com":      true,
	"phonehistory.com":       true,
	"processingbordeaux.com": true,
	"reverselookups.org":     true,
	"usphonepro.com":         true,
	"usphonelookup.com":      true,
	"validnumber.com":        true,
	"whoseno.com":            true,

	// ── Strategy 6: Profile brokers ──────────────────────────────────
	"bradylist.com":         true,
	"docplayer.net":         true,
	"identiq.com":           true,
	"joblookup.com":         true,
	"newcon.com":            true,
	"prehired.io":           true,
	"slideplayer.com":       true,
	"usvisainformation.com": true,
	"facecheck.id":          true,
}

// ValidateURL enforces the allowlist before any browser navigation.
// Returns a descriptive error if the URL is not permitted.
//
// Rules enforced:
//  1. URL must be parseable
//  2. Scheme must be http or https
//  3. Host (stripped of www.) must be in AllowedDomains
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("cannot parse URL %q: %w", rawURL, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("scheme %q not permitted — only http/https allowed", u.Scheme)
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host == "" {
		return fmt.Errorf("URL %q has no host", rawURL)
	}
	if !AllowedDomains[host] {
		return fmt.Errorf("domain %q is not in the broker allowlist — navigation refused", host)
	}
	return nil
}
