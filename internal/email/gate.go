package email

// Verdict is what the presence gate decides about a deletion request.
type Verdict int

const (
	// Allow: the subject is known to be listed there, so the request has a
	// purpose.
	Allow Verdict = iota
	// Warn: nobody has established whether the subject is listed. The request
	// may still be worth making, but it is being made blind.
	Warn
	// Refuse: the subject is known not to be on the site, or the site's
	// category cannot apply to the subject. A deletion request there asks for
	// nothing and hands the subject's name and address to a company that did
	// not have them.
	Refuse
)

// ProxySourcePrefix marks a contact that came from a WHOIS privacy-proxy
// forwarder (see the whois command). A forwarder relays to whoever registered
// the domain, which is not the same as a company's privacy desk, and the tag
// exists so the two are never confused.
const ProxySourcePrefix = "whois-proxy:"

// IsProxyContact reports whether a recorded contact source is a WHOIS
// privacy-proxy forwarder. The default email sweep skips these: a request
// sent to a forwarder is mail to an intermediary that may relay it to the
// registrant or may not, and nothing here can tell which. A person can still
// target one explicitly with --broker.
func IsProxyContact(source string) bool {
	return len(source) >= len(ProxySourcePrefix) && source[:len(ProxySourcePrefix)] == ProxySourcePrefix
}

// PresenceGate decides whether a request may be drafted or sent, from the
// broker's recorded presence finding (the string values of db.Presence; this
// package deliberately does not import db).
//
// It exists because the email path used to ignore presence entirely. Three
// requests went out on 2026-09-23 before any presence check had run, and one of
// those sites was later found to hold no record of the subject at all.
//
// Absent outranks the ability to send: an absent finding comes from a search
// that cleared a control-experiment bar, so it is the strongest statement the
// project has. Unknown values fall through to Warn rather than Allow, so a
// misspelt or future presence value can never silently open the gate.
func PresenceGate(presence string) (Verdict, string) {
	switch presence {
	case "present":
		return Allow, ""
	case "absent":
		return Refuse, "a search found no record of the subject there; a deletion request would ask for nothing and disclose contact details"
	case "not_applicable":
		return Refuse, "this site's category cannot apply to the subject"
	case "undetermined":
		return Warn, "the presence check for this site could not be trusted either way"
	case "":
		return Warn, "this site has never been checked for a record of the subject"
	default:
		return Warn, "unrecognised presence value " + presence
	}
}
