package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

// WhoisContact is an address found in a WHOIS record, with a judgement about
// whether it is any use for a data-deletion request.
// Tier describes how close an address gets to the people running the site.
type Tier string

const (
	TierDirect    Tier = "direct"    // a mailbox at the site's own domain
	TierForwarded Tier = "forwarded" // privacy proxy that relays to the registrant
	TierRegistrar Tier = "registrar" // the registrar's own desk - not the operator
	TierUnknown   Tier = "unknown"   // some other third party
)

type WhoisContact struct {
	Email    string
	Role     string // registrant / admin / tech / abuse / unlabelled
	Tier     Tier
	SameSite bool // the address belongs to the target domain itself
	Usable   bool // worth sending to, directly or via a forwarder
	Why      string
}

// WhoisResult is what one lookup produced.
type WhoisResult struct {
	Domain   string
	Server   string
	Contacts []WhoisContact
	Redacted bool
	// ContactFormURL is set when the registrant field holds a web form
	// instead of an address. Several large registrars now publish one - it is
	// a genuine channel to the registrant, just not an email one, so
	// reporting it as "no contact" hides a route that exists.
	ContactFormURL string
	Raw            int // bytes of response, for sanity
}

var (
	whoisEmailRe = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	contactFormRe = regexp.MustCompile(`(?im)^\s*(?:registrant|admin)\s+(?:email|contact)[^:]*:\s*(https?://\S+)\s*$`)
	// IANA's TLD records name the authoritative server in a "whois:" field
	// (padded with spaces); registry responses sometimes use "refer:" or
	// "Registrar WHOIS Server:" instead. Matching only "whois server:" found
	// none of them and reported "no whois server published for .com".
	registrarReferRe = regexp.MustCompile(`(?im)^\s*Registrar WHOIS Server:\s*(\S+)\s*$`)
	whoisReferRe = regexp.MustCompile(`(?im)^\s*(?:refer|whois|registrar whois server):\s+(\S+)\s*$`)
	redactedRe   = regexp.MustCompile(`(?i)redacted for privacy|data protected|privacy service|gdpr masked|not disclosed|withheld for privacy|statutory masking`)
)

// forwardingProxies relay mail to the real registrant. The address is usually
// an opaque hash at the proxy's domain, and mail sent to it reaches the person
// who registered the domain - indirectly, possibly filtered, but genuinely.
//
// These are kept separate from registrar abuse desks, which they superficially
// resemble. Treating both as dead ends turned ten sites into "no options" when
// what they actually are is "one weak option each", and that difference is the
// whole answer to whether a strategy is worth running.
var forwardingProxies = []string{
	"withheldforprivacy.com", "whoisguard.com", "domainsbyproxy.com",
	"privacyprotect.org", "contactprivacy.com", "identity-protect.org",
	"perfectprivacy.com", "privacyguardian.org", "anonymize.com",
	"domainprivacygroup.com", "registrantprivacy.com", "protecteddomainservices.com",
}

// registrarMailboxes belong to the registrar itself. An abuse desk exists to
// receive complaints about a domain, not requests addressed to the business
// operating the site, and a deletion request sent there reaches a company with
// no relationship to the subject's data.
var registrarMailboxes = []string{
	"namecheap.com", "godaddy.com", "tucows.com", "gandi.net",
	"cloudflare.com", "markmonitor.com", "csc-global.com", "enom.com",
	"networksolutions.com", "dynadot.com", "porkbun.com", "nic.ru",
	"registrar-servers.com", "1api.net", "key-systems.net", "ionos.com",
	"squarespace.com", "wildwestdomains.com",
}

// Whois looks up a domain and reports the contact addresses in the record.
//
// It resolves the authoritative server through IANA rather than guessing one
// per TLD, then queries that server directly on port 43.
//
// A caution that belongs with every result: since GDPR took effect, registrant
// contacts in WHOIS are redacted or proxied for most commercial domains. A
// CCPA deletion request sent to a registrar's abuse desk or a privacy
// forwarding service is not a request to the data broker - it is noise sent to
// an uninvolved third party. So an address is only reported as usable when it
// belongs to the target domain itself.
func Whois(ctx context.Context, domain string) (*WhoisResult, error) {
	res := &WhoisResult{Domain: domain}

	tld := domain
	if i := strings.LastIndex(domain, "."); i >= 0 {
		tld = domain[i+1:]
	}

	ianaBody, err := whoisQuery(ctx, "whois.iana.org", tld)
	if err != nil {
		return res, fmt.Errorf("iana lookup for .%s: %w", tld, err)
	}
	server := ""
	for _, line := range strings.Split(ianaBody, "\n") {
		if m := whoisReferRe.FindStringSubmatch(line); m != nil {
			server = m[1]
			break
		}
	}
	if server == "" {
		return res, fmt.Errorf("no whois server published for .%s", tld)
	}
	res.Server = server

	body, err := whoisQuery(ctx, server, domain)
	if err != nil {
		return res, fmt.Errorf("query %s: %w", server, err)
	}

	// Follow the registrar referral. For .com the registry (Verisign) returns
	// a thin record: the registrar's name, its abuse desk, and a pointer. The
	// registrant contact - including the privacy-proxy forwarder that is the
	// only real channel to most of these operators - lives at the registrar's
	// own server. Stopping at the registry saw one address per domain, all of
	// them registrar abuse desks, and concluded that ten sites had no contact
	// of any kind.
	if ref := registrarReferRe.FindStringSubmatch(body); ref != nil {
		next := strings.TrimSpace(ref[1])
		if next != "" && !strings.EqualFold(next, server) {
			if deeper, dErr := whoisQuery(ctx, next, domain); dErr == nil && len(deeper) > len(body)/2 {
				res.Server = next
				body = deeper
			}
		}
	}

	res.Raw = len(body)
	res.Redacted = redactedRe.MatchString(body)

	if m := contactFormRe.FindStringSubmatch(body); m != nil {
		res.ContactFormURL = strings.TrimSpace(m[1])
	}

	seen := map[string]bool{}
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		for _, addr := range whoisEmailRe.FindAllString(line, -1) {
			key := strings.ToLower(strings.Trim(addr, ".,;:()<>\"'"))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			res.Contacts = append(res.Contacts, classifyWhoisEmail(key, line, domain))
		}
	}
	sort.SliceStable(res.Contacts, func(i, j int) bool {
		rank := map[Tier]int{TierDirect: 0, TierForwarded: 1, TierUnknown: 2, TierRegistrar: 3}
		return rank[res.Contacts[i].Tier] < rank[res.Contacts[j].Tier]
	})
	return res, nil
}

func classifyWhoisEmail(addr, context, domain string) WhoisContact {
	c := WhoisContact{Email: addr, Role: "unlabelled"}

	low := strings.ToLower(context)
	switch {
	case strings.Contains(low, "registrant"):
		c.Role = "registrant"
	case strings.Contains(low, "admin"):
		c.Role = "admin"
	case strings.Contains(low, "tech"):
		c.Role = "tech"
	case strings.Contains(low, "abuse"):
		c.Role = "abuse"
	}

	emailDomain := addr[strings.LastIndex(addr, "@")+1:]
	base := baseDomain(domain)
	c.SameSite = emailDomain == base || strings.HasSuffix(emailDomain, "."+base)

	if c.SameSite {
		c.Tier = TierDirect
		c.Usable = true
		c.Why = "mailbox at the site's own domain"
		if c.Role == "abuse" {
			c.Why = "abuse desk at the site's own domain - may not route a rights request"
		}
		return c
	}

	for _, p := range forwardingProxies {
		if emailDomain == p || strings.HasSuffix(emailDomain, "."+p) {
			c.Tier = TierForwarded
			c.Usable = true
			c.Why = "privacy-proxy forwarder - relays to the registrant, so it reaches the operator indirectly"
			return c
		}
	}
	for _, r := range registrarMailboxes {
		if emailDomain == r || strings.HasSuffix(emailDomain, "."+r) {
			c.Tier = TierRegistrar
			c.Why = "registrar mailbox - a company with no relationship to your data"
			return c
		}
	}

	c.Tier = TierUnknown
	c.Why = "third-party domain - unclear whether it reaches the operator"
	return c
}

// baseDomain trims a leading www. so "www.example.com" and "example.com"
// compare equal.
func baseDomain(d string) string {
	return strings.TrimPrefix(strings.ToLower(d), "www.")
}

// Best returns the most promising usable contact, or nil.
// Best returns the closest usable contact: a mailbox at the site's own domain
// if there is one, otherwise a forwarder that relays to the registrant.
func (r *WhoisResult) Best() *WhoisContact {
	for _, want := range []Tier{TierDirect, TierForwarded} {
		for i := range r.Contacts {
			if r.Contacts[i].Tier == want && r.Contacts[i].Usable {
				return &r.Contacts[i]
			}
		}
	}
	return nil
}

func whoisQuery(ctx context.Context, server, query string) (string, error) {
	d := net.Dialer{Timeout: 12 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := conn.Write([]byte(query + "\r\n")); err != nil {
		return "", err
	}
	b, err := io.ReadAll(conn)
	if err != nil && len(b) == 0 {
		return "", err
	}
	return string(b), nil
}
