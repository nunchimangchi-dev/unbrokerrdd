# unbrokerrdd

An AI-driven data broker opt-out orchestrator. It seeds a queue of 85 real US data brokers, walks each one through an opt-out strategy suited to that site, and tracks progress on a real-time dashboard.

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
- Full broker registry — 85 real data brokers, categorized into 6 opt-out strategy types, seeded into SQLite.
- Strategy 1 (TruthFinder affiliate cascade) — fully implemented and tested against the real site. One form submission at `truthfinder.com/privacy-center` covers 7 affiliated broker properties; Claude Haiku (vision) validates the outcome from a post-submit screenshot.
- Orchestrator with QA-gated batch execution, SQLite-backed state (`pending → in_progress → success/failed/skipped/manual`), and a CLI (`serve`, `run`, `status`, `reset`).
- Real-time dashboard (Go HTTP + WebSocket server on `:8080`) with a dark, neon aesthetic and a subtle Three.js background effect.

**Planned, not yet built:**
- Strategies 2–6 (hidden opt-out forms, privacy-page email discovery, business directory checks, WHOIS-based phone directory removal, and profile-broker account deletion) — the broker registry already has all 85 sites categorized into these buckets, but only Strategy 1's execution agent exists. See `CLAUDE.md` for the per-strategy breakdown and status.
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

## Security & compliance

[`docs/SECURITY-BASELINE.md`](docs/SECURITY-BASELINE.md) — audit results,
what's clean, and what's open, adapted from the skyrise project standard to
this project's actual shape (single-user local tool, not a hosted service).

## How this was built

Built iteratively with heavy AI involvement (Claude, with earlier drafts assisted by Gemini) across a number of model generations — expect some rough edges and inconsistency between older and newer parts of the codebase. `CLAUDE.md` is kept in the repo as the actual working brief used during development, left in for transparency rather than trimmed out.

## License

MIT — see [LICENSE](LICENSE).
