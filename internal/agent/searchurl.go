package agent

// SearchTemplates maps a broker ID to a URL template that runs a name search on
// that site, for use by the presence check.
//
// THE RULE FOR THIS MAP: a template goes in here only after it has been run
// live and seen to return a real results page. Not from the site's docs, not
// from a plausible-looking URL pattern, not from another broker on the same
// platform. This project has three separate incidents on record of a site being
// classified from indirect evidence and being wrong for days each time - see
// the table in CLAUDE.md. A guessed search URL is worse than no entry at all,
// because a 404 renders as "no results", which classifies as absent, which
// tells the subject they are not listed on a site that may well list them.
//
// A broker with no entry here is simply reported as having no verified search
// path, which is an honest "not checked", and the presence check moves on.
//
// Placeholders: {name} {first} {last} {first+last} - all query-escaped.
var SearchTemplates = map[string]string{
	// Verified live on 2026-09-23. See PRESENCE.md for the verification record.
}

// SearchTemplateFor returns the verified search template for a broker ID.
func SearchTemplateFor(brokerID string) (string, bool) {
	t, ok := SearchTemplates[brokerID]
	return t, ok
}
