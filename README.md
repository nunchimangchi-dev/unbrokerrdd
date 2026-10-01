# unbrokerrdd

An AI-driven data broker opt-out orchestrator. It seeds a queue of real US data brokers, walks each one through an opt-out strategy suited to that site, and tracks progress on a real-time dashboard.

**For current, accurate numbers — registry size, how many are actually done, and how many of those the automation completed on its own versus a human — run `databrokergo status`.** Counts are deliberately not hardcoded in this README or anywhere else: every hand-maintained copy of them drifted out of date, and reconciling them after the fact was its own source of error.

Built as a genuine attempt at solving a real privacy problem with an AI agent doing the meaningful decision-making — not a script with an LLM bolted on.

## Why it exists

I wanted my own data off the data-broker sites, and expected that to be simple. It wasn't. The effort only became visible as the tool was built: sites that block automated browsers, dead or seized domains, opt-outs that first require finding your own listing, and confirmation emails that say nothing about whether anything was removed.

The project was not built to demonstrate that difficulty. It became a record of it, and it made me think about how hard this must be for someone working through these sites by hand.

That is one person's experience, and it should be read as such. The tool deliberately does not try to defeat CAPTCHAs or bot detection, so part of what it found difficult is a choice. Where automation is blocked it routes the task to a human and records that it did.

## What this can and can't establish

- **A completed request is not a confirmed removal.** Broker confirmation emails do not report the state of the removal, and the Gmail grant is compose-only, so the tool cannot read replies. The database records that a request was made, and by whom, not that a listing came down.
- **"Not listed" cannot be attributed.** Commercial removal services were used before this tool ran, so a later "absent" finding may mean the subject was never listed, was removed by one of those services, or was removed by this tool. There is no baseline from before.
- **Autonomous completion has not been demonstrated.** `completion_method` is `autonomous` only after an unattended live run. `databrokergo status` shows the current split.
- **It is one subject on one machine.** Nothing here is a general measure of how the data-broker ecosystem behaves.

## About this commit history

The history here is intentionally unredacted. It records the bugs as well as
the features, including ones that were mine and embarrassing: a browser-profile
leak that filled a tmpfs until no sandboxed application on the machine could
start, a classifier that confidently reported "you are not listed on this site"
about a page that had never run the search, a reachability sweep that declared
the California Secretary of State a dead domain, and a draft-listing feature
that read far more of a mailbox than it had any business touching.

Each of those was found, explained, fixed, and covered by a test that would
catch it again — and the commit message says which. That record is kept rather
than squashed because a curated history would make this project look more
finished and be worth less. The whole point of the tool is refusing to report a
removal it did not earn; a repository that quietly hid its own failures while
doing so would be making exactly the mistake it exists to prevent.

If you are evaluating this as work: read `databrokergo status` output and the
commit messages together. The first says what is true right now, the second
says what it cost to find out.

## Before you run this

**This costs real money, on your own Anthropic account.** Every non-dry-run
broker submission calls Claude Haiku's vision API once, to validate the
outcome from a post-submit screenshot — billed to whatever
`ANTHROPIC_API_KEY` you supply. Check
[Anthropic's current pricing](https://www.anthropic.com/pricing) before
running this against real brokers; the exact cost depends on their pricing
at the time you run it, not anything fixed here. `--dry-run` is completely
free — it returns before ever calling the API or opening a browser, so
explore the tool with it first.

**A fragment of your PII leaves your machine, by design.** The validation
screenshot is taken *after* your name/email have been entered into the
broker's form, so it can visibly contain them — that screenshot gets sent
to Anthropic's API as part of normal operation. This tool follows a
minimal-data principle (see `.env.example` — no SSN, no DOB, nothing beyond
what a given opt-out legally requires), which limits the blast radius, but
doesn't eliminate it: your name and email are shared with Anthropic as a
third party every time a broker submission actually runs. If that's not
acceptable to you, `--dry-run` doesn't have this exposure at all, since it
never takes a screenshot or calls the API.

## Current state

This section describes build state only. For counts, and for what has actually been completed and by whom, run `databrokergo status`. `CLAUDE.md` has the per-strategy table, and `HANDOFF.md` records what was tried and what blocked it.

**Working today:**
- Full broker registry — real US data brokers categorized into 6 opt-out strategy types, seeded into SQLite (`databrokergo status` for the live count and completion breakdown).
- Strategy 1 (PeopleConnect suppression portal) — built. One request at `suppression.peopleconnect.us` (PeopleConnect's shared portal, not `truthfinder.com/privacy-center`'s account-deletion tool — that one doesn't suppress your public listing, corrected 2026-09-17, see `HANDOFF.md`) is intended to cover the affiliated brokers. That cascade has not been independently verified. Claude Haiku (vision) validates the outcome from a post-submit screenshot.
- Strategy 3 (privacy-page discovery and CCPA email) — built. `discover` finds a published contact address and `email` drafts a deletion request. Drafting is the default and sending needs two flags. See `GMAIL.md`.
- Presence checks — a search only counts once it has passed a two-sided control experiment (`PRESENCE.md`).
- Orchestrator with QA-gated batch execution, SQLite-backed state (`pending → in_progress → success/failed/skipped/manual`), a 48h cooldown, and a CLI for running, classifying and inspecting targets.
- Real-time dashboard (Go HTTP + WebSocket server on `:8080`) with a dark, neon aesthetic and a subtle Three.js background effect.

**Retired:**
- Strategy 5 (WHOIS phone directories) is obsolete: registrars now redact the registrant contact the strategy depended on. The evidence is recorded in code and printed by `status`.
- Orchestrator with QA-gated batch execution, SQLite-backed state (`pending → in_progress → success/failed/skipped/manual`), and a CLI (`serve`, `run`, `status`, `reset`).
- Real-time dashboard (Go HTTP + WebSocket server on `:8080`) with a dark, neon aesthetic and a subtle Three.js background effect.

**Partially built:**
- Strategy 2 (hidden opt-out forms) has two verified, working sites — CheckPeople (2026-09-17) and AdvancedBackgroundChecks (2026-09-18). Every other Strategy 2 broker is deliberately routed to a manual status rather than automated with unverified selectors. A live sweep of the rest of the BADBOOL manual list (2026-09-18) found real, varied blockers: FamilyTreeNow, Clustal, and Nuwber sit behind bot-check interstitials that either got chromedp visibly stuck (`FamilyTreeNow`) or explicitly require reCAPTCHA (`Nuwber`); Spokeo, Clustal, BeenVerified, and SmartBackgroundChecks require the human to find and paste their own listing's URL first (a correctness requirement, not a missing feature — auto-selecting "which search result is you" risks opting out a stranger's data); That's Them needs a street address and phone number the tool deliberately doesn't collect. One piece of good news found along the way: Radaris.com was seized by New Jersey court order in August 2026 over Daniel's Law violations and no longer operates as a people-search site at all — nothing to opt out of there anymore.

**Planned, not yet built:**
- Strategy 4 (business directory checks) and Strategy 6 (profile-broker account deletion). Both are seeded in the registry with no execution agent, and Strategy 6's targets turned out to be largely different from what the registry assumed. See `CLAUDE.md` for the per-strategy breakdown.

## Components

- **`databrokergo/`** — the whole project. A Go backend (CLI + orchestrator + dashboard server) with a vanilla-JS/Three.js frontend, no build step.
  - `cmd/databrokergo/` — CLI entry point (`serve`, `run`, `status`, `reset`)
  - `internal/orchestrator/` — batch management and QA gates
  - `internal/agent/` — Claude Haiku vision validation, domain allowlist
  - `internal/db/` — SQLite state store
  - `internal/dashboard/` — broker registry, HTTP/WebSocket server, static frontend
  - `strategies/` — one file per opt-out strategy (currently: Strategy 1 only)
- **`internal/dashboard/static/databroker2.html`** — an earlier, standalone dashboard mockup (Gemini-generated), kept as a historical artifact rather than the live dashboard. The live dashboard is `index.html` in the same directory.

## Running it

```bash
cp .env.example .env   # fill in SUBJECT_NAME, SUBJECT_EMAIL, SUBJECT_STATE, ANTHROPIC_API_KEY

go run ./cmd/databrokergo serve                          # dashboard at http://localhost:8080
go run ./cmd/databrokergo run --strategy 1 --dry-run      # dry-run Strategy 1
go run ./cmd/databrokergo status                          # print current broker status table
go run ./cmd/databrokergo reset --broker "backgroundcheckme"
```

`ANTHROPIC_API_KEY` can also be supplied via the macOS Keychain instead of `.env` — see `internal/config/config.go`.

The dashboard binds to `127.0.0.1` only.

**If a live run fails with something like `exec: "google-chrome": executable file not found`:** chromedp looks for Chrome under a fixed list of binary names on `PATH` (`chromium`, `google-chrome`, etc.) and won't find a Flatpak-only install. Fix with a one-time shim (outside the repo, machine-local, not something `go build` or CI needs):
```bash
mkdir -p ~/.local/bin
cat > ~/.local/bin/google-chrome << 'SHIM'
#!/bin/sh
exec flatpak run --die-with-parent com.google.Chrome "$@"
SHIM
chmod +x ~/.local/bin/google-chrome
```
**The `--die-with-parent` flag is not optional.** Without it, `flatpak run` detaches from the sandboxed Chrome process in a way that reparents it away from chromedp entirely — `cancel()` kills nothing, and the whole Chrome process tree (browser, GPU process, zygotes, network service) leaks forever after every single run. Verified this the hard way 2026-09-17: two full leaked Chrome process trees from real runs were still running *hours* later, and killing them by PID pattern (`pkill -f`) unreliably failed against the nested sandbox — only killing the exact outer `bwrap` PIDs worked. `--die-with-parent` fixes the leak at the source; confirmed clean with a before/after process check (zero leftover processes immediately after `cancel()`, vs. a full tree still alive hours later without the flag).

If you already created the shim before 2026-09-17, regenerate it with the command above — the old version leaks a Chrome process tree on every run.

## Security & compliance

[`docs/SECURITY-BASELINE.md`](docs/SECURITY-BASELINE.md) — audit results,
what's clean, and what's open, adapted from the skyrise project standard to
this project's actual shape (single-user local tool, not a hosted service).

## How this was built

Built iteratively with heavy AI involvement (Claude, with earlier drafts assisted by Gemini) across a number of model generations — expect some rough edges and inconsistency between older and newer parts of the codebase. `CLAUDE.md` is kept in the repo as the actual working brief used during development, left in for transparency rather than trimmed out.

## License

MIT — see [LICENSE](LICENSE).
