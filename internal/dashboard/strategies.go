package dashboard

// StrategyState is the build-and-viability state of a strategy.
//
// "Obsolete" is deliberately a state of its own rather than deletion. A
// strategy that is removed from the code leaves no trace of why, so the next
// person to look at the target list sees twelve phone directories with no
// handler and reasonably concludes that someone simply never got to them. They
// then rebuild the same thing and rediscover the same wall. Keeping the
// strategy with its rationale and the evidence that retired it costs a few
// lines and answers the question before it is asked again.
type StrategyState string

const (
	StrategyBuilt    StrategyState = "built"
	StrategyPartial  StrategyState = "partial"
	StrategyPlanned  StrategyState = "planned"
	StrategyObsolete StrategyState = "obsolete"
)

// StrategyInfo describes one strategy and how far it can actually get.
type StrategyInfo struct {
	N         int
	Name      string
	Mechanism string
	State     StrategyState
	// Note explains the state. For an obsolete strategy it must say what
	// changed in the world, not merely that the strategy failed.
	Note string
	// Evidence is the measurement behind the state, with the date it was
	// taken, so the claim can be re-checked rather than believed.
	Evidence string
}

// Strategies is the canonical account of what each strategy can do.
func Strategies() []StrategyInfo {
	return []StrategyInfo{
		{
			N: 1, Name: "TruthFinder Affiliates",
			Mechanism: "one suppression request at PeopleConnect's shared portal",
			State:     StrategyBuilt,
			Note:      "one submission cascades to the whole affiliate bucket",
		},
		{
			N: 2, Name: "Hidden Opt-Out Pages",
			Mechanism: "per-site browser handler: navigate, fill, validate",
			State:     StrategyPartial,
			Note:      "a router, not one mechanism - each site needs live-verified selectors; unverified IDs route to manual rather than being guessed at",
		},
		{
			N: 3, Name: "Privacy Page Discovery",
			Mechanism: "follow the site's privacy links, extract a contact address, send a CCPA request",
			State:     StrategyBuilt,
			Note:      "works, but most brokers publish a web form rather than an address",
			Evidence:  "2026-09-23: of 25 sites, 3 published a usable address, 16 published none, 3 failed TLS, 3 were dead",
		},
		{
			N: 4, Name: "Business Directories",
			Mechanism: "search the subject's name, skip if absent",
			State:     StrategyPlanned,
			Note:      "needs the presence check to have verified search templates; none has passed the control experiment yet",
		},
		{
			N: 5, Name: "Phone Directories (WHOIS)",
			Mechanism: "WHOIS lookup for a registrant address, then send a CCPA request",
			State:     StrategyObsolete,
			Note: "GDPR ended the premise. This strategy assumed WHOIS exposes a " +
				"registrant address; registrars now redact it by default and publish a " +
				"privacy proxy or a web contact form instead. The lookup is built and " +
				"still runs - `databrokergo whois` - because a proxy forwarder does " +
				"relay to the registrant and is worth recording. But WHOIS is no longer " +
				"a route to a broker's privacy contact, and no amount of further work on " +
				"this strategy changes that, because what changed is the registry system, " +
				"not the code.",
			Evidence: "2026-09-23: of 10 live domains, 0 published a mailbox at their own domain, 3 exposed a privacy-proxy forwarder, 5 published a web contact form, 2 exposed only their registrar's abuse desk",
		},
		{
			N: 6, Name: "Profile Brokers",
			Mechanism: "account-based deletion, or flag for a human",
			State:     StrategyPlanned,
			Note:      "not started; several targets in this bucket turned out to be dead domains",
		},
	}
}

// StrategyByN returns one strategy's info.
func StrategyByN(n int) (StrategyInfo, bool) {
	for _, s := range Strategies() {
		if s.N == n {
			return s, true
		}
	}
	return StrategyInfo{}, false
}
