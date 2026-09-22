# unbrokerrdd

An AI-driven data broker opt-out orchestrator. It seeds a queue of real US data brokers, walks each one through an opt-out strategy suited to that site, and tracks progress on a real-time dashboard.

**For current, accurate numbers — registry size, how many are actually done, and how many of those the automation completed on its own versus a human — run `databrokergo status`.** Counts are deliberately not hardcoded in this README or anywhere else: every hand-maintained copy of them drifted out of date, and reconciling them after the fact was its own source of error.

Built as a genuine attempt at solving a real privacy problem with an AI agent doing the meaningful decision-making — not a script with an LLM bolted on.

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

**Working today:**
- Full broker registry — real US data brokers categorized into 6 opt-out strategy types, seeded into SQLite (`databrokergo status` for the live count and completion breakdown).
- Strategy 1 (TruthFinder affiliate cascade) — fully implemented and tested against the real site. One suppression request at `suppression.peopleconnect.us` (PeopleConnect's shared portal, not `truthfinder.com/privacy-center`'s account-deletion tool — that one doesn't suppress your public listing, corrected 2026-09-17, see `HANDOFF.md`) covers 7 affiliated broker properties; Claude Haiku (vision) validates the outcome from a post-submit screenshot.
- Orchestrator with QA-gated batch execution, SQLite-backed state (`pending → in_progress → success/failed/skipped/manual`), and a CLI (`serve`, `run`, `status`, `reset`).
- Real-time dashboard (Go HTTP + WebSocket server on `:8080`) with a dark, neon aesthetic and a subtle Three.js background effect.

**Partially built:**
- Strategy 2 (hidden opt-out forms) has two verified, working sites — CheckPeople (2026-09-17) and AdvancedBackgroundChecks (2026-09-18). Every other Strategy 2 broker is deliberately routed to a manual status rather than automated with unverified selectors. A live sweep of the rest of the BADBOOL manual list (2026-09-18) found real, varied blockers: FamilyTreeNow, Clustal, and Nuwber sit behind bot-check interstitials that either got chromedp visibly stuck (`FamilyTreeNow`) or explicitly require reCAPTCHA (`Nuwber`); Spokeo, Clustal, BeenVerified, and SmartBackgroundChecks require the human to find and paste their own listing's URL first (a correctness requirement, not a missing feature — auto-selecting "which search result is you" risks opting out a stranger's data); That's Them needs a street address and phone number the tool deliberately doesn't collect. One piece of good news found along the way: Radaris.com was seized by New Jersey court order in August 2026 over Daniel's Law violations and no longer operates as a people-search site at all — nothing to opt out of there anymore.

**Planned, not yet built:**
- Strategies 3–6 (privacy-page email discovery, business directory checks, WHOIS-based phone directory removal, and profile-broker account deletion) — the broker registry already has all sites categorized into these buckets, but no execution agent exists yet. See `CLAUDE.md` for the per-strategy breakdown and status.
- Gmail integration for the CCPA email-based strategies (3 and 5) — needs real Gmail API (OAuth2) access; not implemented.

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
