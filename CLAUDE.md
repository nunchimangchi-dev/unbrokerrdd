# databrokergo

**Purpose:** An AI agent system for removing personal data from real US data brokers. A genuine attempt at a real privacy problem, not a script with an LLM bolted on.

**Be precise about what "automated" means here.** Several target sites actively detect and block automated browser sessions, and this project does not attempt to defeat CAPTCHAs or bot-detection — a deliberate line, see HANDOFF.md. Where automation is blocked, the tool routes to a human-completed path rather than recording a success it didn't earn. The database records which actually happened: `completion_method` is `autonomous` only when the agent completed a removal unattended on a live run, and nothing except a successful live run can set it.

**Do not hand-write broker counts or automation claims into docs.** Every hand-maintained copy of those numbers has drifted out of date (85 / 86 / 87 / 95 have all been "current" simultaneously in different files). `databrokergo status` is the authority for registry size, database state, orphaned rows, status breakdown, and the autonomous-vs-human completion split.

**Owner:** you@example.com

---

## What this is

An AI-powered data broker opt-out orchestrator. It:
1. Loops through a queue of data brokers (SQLite-backed)
2. Dispatches work to specialized agents based on the broker's strategy type
3. Updates a real-time dashboard (3D pixel art, localhost:8080) via WebSocket
4. Runs incrementally — each batch is QA-gated before the next one starts

Brokers and their strategy assignments live inline in `internal/dashboard/brokers.go` — that's the canonical registry (no separate registry file). The registry declares intent; the SQLite database holds live state. They can drift apart (`Seed` is INSERT OR IGNORE, so removing a broker from code never deletes its row) — `databrokergo status` reports orphans where they disagree.

---

## Architecture

```
cmd/databrokergo/main.go        ← CLI entry: `serve`, `run`, `status`, `reset`
internal/
  orchestrator/
    runner.go     ← batch management, loop control, QA gates
  agent/
    claude.go     ← Claude Haiku vision — validates opt-out outcomes from screenshots
    allowlist.go  ← domain allowlist for agent-driven navigation
    types.go      ← Agent, Result types
  config/
    config.go     ← .env loading + macOS Keychain fallback for the Anthropic key
  db/
    store.go      ← SQLite state: broker status, attempt log, timestamps
  dashboard/
    brokers.go    ← canonical broker registry + strategy assignments
    server.go     ← Go HTTP + WebSocket server on :8080
    static/       ← frontend: dashboard UI (index.html + aether-core.css)
strategies/
  strategy1_truthfinder.go      ← 7 sites, 1 form submission — IMPLEMENTED
  strategy2_*.go                ← per-site handlers (checkpeople, advancedbackgroundchecks, spokeo)
  (strategies 3-6: planned, not yet built — see below)
```

Note: earlier drafts of this doc described a broader file layout (`agent/browser.go`, `agent/email.go`, `agent/whois.go`, a `databrokergo.md` registry file) that was never actually built — this section now reflects what's really on disk.

---

## The 6 Strategies

Per-strategy site counts are intentionally omitted here — run `databrokergo status`
for live numbers. Only the build state is tracked in this table.

| # | Name | Mechanism | Build state |
|---|------|-----------|-------------|
| 1 | TruthFinder Affiliates | 1 suppression request at suppression.peopleconnect.us (PeopleConnect's shared portal), chromedp + Claude Haiku vision validation | **Built** — corrected 2026-09-17, see HANDOFF.md |
| 2 | Hidden Opt-Out Pages | Per-site handler: navigate to opt-out URL, fill form, validate | **Partially built** — handlers for `checkpeople`, `advancedbackgroundchecks`, `spokeo`; every other broker.ID routes to manual |
| 3 | Privacy Page Discovery | Scrape contact email → send CCPA email | Planned — needs Gmail OAuth2, not started |
| 4 | Business Directories | Search name → skip if not found | Planned — not started |
| 5 | Phone Directories (WHOIS) | WHOIS lookup → send CCPA email | Planned — needs Gmail OAuth2, not started |
| 6 | Profile Brokers | Account-based deletion or flag manual | Planned — not started |

Strategy 1's agent covers its whole bucket (one PeopleConnect suppression submission cascades to the affiliates — see the 2026-09-17 correction in `HANDOFF.md`, it used to hit the wrong TruthFinder control entirely).

Strategy 2 is a **router, not a single mechanism** — each broker.ID needs its own live-verified selectors, because real sites vary wildly (cookie banners, iframes, hidden inputs behind styled labels, bot-check interstitials, search-and-select-your-listing steps). Any broker.ID without a verified handler routes to `StatusManual` rather than being guessed at. Two cautionary examples, both of which cost real debugging time:

- **Don't infer a blocker from a failure mode.** CheckPeople timed out for days and was classified `bot_defended` by pattern-matching to Strategy 1's genuine bot-detection. The real cause was a zero-size hidden checkbox that needed its wrapping `<label>` clicked. It was never a defense at all.
- **Don't classify a site without loading it.** Five sites were marked `needs_profile_url` from documentation alone; when actually checked, four turned out to be bot-defended and only Spokeo was genuinely clean.

See the 2026-09-17, 09-18, and 09-22 entries in `HANDOFF.md` for what's been tried and what really blocks the rest.

---

## AI Model Usage (cost-conscious)

| Task | Model | Why | Status |
|------|-------|-----|--------|
| Opt-out success validation | claude-haiku-4-5 | Binary decision from a screenshot, fast | **Built** |
| Form field detection | claude-haiku-4-5 | High volume, simple classification | Planned (Strategy 2) |
| Privacy page email extraction | claude-haiku-4-5 | Pattern matching | Planned (Strategy 3) |
| CCPA email drafting | claude-sonnet-4-6 | Needs to sound legitimate and professional | Planned (Strategy 3/5) |
| Anomaly / edge case handling | claude-sonnet-4-6 | Judgment call, infrequent | Planned |

Always use prompt caching (`cache_control: ephemeral`) on system prompts. Never use Opus for automated batch work.

---

## Execution Order (Incremental Batches)

Batch 1 (Strategy 1: TruthFinder, 1 submission covering 7 sites) is the only batch that's actually run. The batch plan below for Strategies 2+ is the intended rollout order once those strategies are built — not yet executed:

```
Batch 1 (QA gate)  → Strategy 1: TruthFinder only (1 submission, 7 covered)  ✓ done
Batch 2            → Strategy 2: 3 hidden opt-out pages (hand-picked, confirmed URLs)
Batch 3            → Strategy 2: remaining 11 + Strategy 5: 3 WHOIS sites
Batch 4            → Strategy 3: 10 privacy discovery sites
...
```

**Rule:** No batch starts until the previous batch is validated (success rate logged, failures reviewed).

---

## Status States

```
pending     → not yet attempted
in_progress → agent currently working
success     → opt-out submitted/confirmed
failed      → attempted, error or no opt-out path found
skipped     → nothing to do (no matching record, dead site)
manual      → requires human action (flagged by agent)
```

`status` alone is too coarse to act on — a broker can be `manual` for several
genuinely different reasons, and only some are worth ever retrying. Two more
columns carry that detail:

```
blocker_type       → WHY it's stuck:
  dead_site          domain gone/parked/seized — nothing to do, ever
  bot_defended       active anti-automation defense — do not attempt evasion
  needs_profile_url  requires search-and-select-your-listing first
  no_mechanism       no self-serve opt-out found anywhere on the site
  missing_field      needs PII the tool deliberately doesn't collect
  covered_by_other   resolved via another broker's submission (see covered_by)
  unbuilt            straightforward site, handler just not written yet

completion_method  → HOW a success was achieved:
  autonomous         the agent did it unattended on a live run
  human_completed    a person performed the submission
```

`completion_method` is the project's real scoreboard. Only `Settle` on a
successful live run can set `autonomous`; the `completed` CLI command marks
human work and cannot claim otherwise.

```
presence           → is the subject ON this site at all:
  present            a matching result is listed - removal work is warranted
  absent             the search ran cleanly and found nothing to remove
  undetermined       the check could not be trusted either way
  (empty)            never checked
```

`presence` is a third, independent axis, and it is deliberately not part of the
scoreboard. A site the subject was never listed on is not a removal and must
never be counted as one — but it is also not a failure, and it is the most
common honest outcome across business directories and niche profile sites.
Keeping it in its own column means "how many sites list me" and "how many did
the tool get me off" stay two numbers that cannot be mistaken for each other.

Until these are checked, the registry size is not an exposure figure. `status`
now says so directly. See `PRESENCE.md` for why `absent` has to clear a much
higher bar than the other two findings, and why a search URL only counts once
it has passed a two-sided control experiment.

---

## Dashboard

- **URL:** http://localhost:8080
- **Tech:** Go HTTP server → WebSocket → Three.js 3D pixel art in browser
- **Aesthetic:** Dark background, neon pixel art, each broker is a pixel art "target"
- **Interaction:** Broker tiles animate from gray → yellow (in progress) → green (success) / red (failed)
- **Real-time:** WebSocket pushes status updates as agents complete work
- **Stats bar:** Total brokers, success count, failed count, in-progress count, estimated completion

---

## Key Commands

```bash
go run ./cmd/databrokergo serve         # start dashboard + agent loop
go run ./cmd/databrokergo run --strategy 2 [--dry-run] [--limit N]
go run ./cmd/databrokergo status        # AUTHORITATIVE: registry vs db, orphans,
                                        # status breakdown, autonomous-vs-human split
go run ./cmd/databrokergo reset --broker checkpeople          # back to pending
go run ./cmd/databrokergo blocker --broker nuwber --type bot_defended
go run ./cmd/databrokergo set-profile-url --broker spokeo --url <listing-url>
go run ./cmd/databrokergo completed --broker spokeo --note "..."  # human-completed
go run ./cmd/databrokergo probe --url https://site.com/opt-out   # read-only recon
go run ./cmd/databrokergo probe --broker spokeo
go run ./cmd/databrokergo reach [--limit N] [--apply]            # DNS + browser: is the site alive?
go run ./cmd/databrokergo presence --verify-template --url "<tmpl>" --site X
go run ./cmd/databrokergo presence [--broker X | --strategy N] [--dry-run]
```

**Run `probe` before classifying any site.** It loads the page in the same
headless browser the strategies use and reports what *that* browser receives —
never fills, never submits. Every wrong `blocker_type` on this project came
from judging a site by something else:

| What was used | What it actually told you | Real answer |
|---|---|---|
| WebFetch returned 403 | the site rejects non-browser user agents | locatefamily loads fine in a browser — wrong call |
| handler timed out | *something* went wrong | CheckPeople had a zero-size checkbox needing its label clicked — never a defense |
| page loads in your own Chrome | a human can use it | Spokeo serves chromedp a 403 — wrong call |
| probe reported a nav error | the *browser* failed, not the site | a local Chrome launch failure was reported as `bot_defended` — probe now refuses to conclude anything without a response |
| a name search returned nothing | *something* returned nothing | allpeople.biz ignored the query and served its A–Z index; "no match" was a false all-clear |

`probe` answers all three directly, including naming the label selector to
click when it finds a hidden input.

`run` against a broker with a recent real attempt is refused by a 48h cooldown
(`orchestrator.CooldownWindow`), checked against the permanent attempts log so
it survives a `reset`. That exists because repeated same-day automated traffic
from one IP is what got AdvancedBackgroundChecks CAPTCHA-gated — a self-inflicted
block. Don't work around it; it's pacing, not an obstacle.

---

## Testing Philosophy

- **Every strategy gets a unit test** with a mock HTTP server (no real requests in tests)
- **Integration test** for each strategy uses a local test server that mimics the target site
- **Batch 1 is the QA run** — real execution against TruthFinder only, result manually validated before anything else runs
- **Never run the full batch without a validated dry-run result**

---

## Dependencies (Go modules)

```
github.com/chromedp/chromedp          ← browser automation
modernc.org/sqlite                    ← SQLite (pure Go, no CGo)
github.com/gorilla/websocket          ← WebSocket for dashboard
github.com/anthropics/anthropic-sdk-go ← Claude API
```

Frontend (no npm, vanilla JS):
- Three.js via CDN
- No React, no build step — just static files served by Go

---

## What NOT to do

- Don't use GPT-4 or Gemini — Claude API only
- Don't automate Strategy 7 (DROP platform / Google delisting) — these are one-offs
- Don't run full batch without QA gate passing
- Don't burn Sonnet/Opus tokens on tasks Haiku can handle
- Don't use a frontend framework — keep the dashboard as vanilla JS + Three.js
- Don't store personal data (name, address) in code — load from env vars or a local secrets file

---

## Personal Data Config

Stored in `.env` (gitignored), never committed:
```
SUBJECT_NAME="Full Name"
SUBJECT_EMAIL="you@example.com"
SUBJECT_ADDRESS="..."
SUBJECT_STATE="OH"
```

---

## Gmail Integration — Planned, Not Built

Strategies 3 and 5 will need to send real CCPA deletion-request emails and check for replies. Not implemented yet: `config.go` only has a `GMAIL_SENDER` field, no send/read logic exists.

Note for whoever builds this: Gmail MCP tools (as used inside a Claude Code session) aren't reachable from a standalone compiled Go binary at runtime — that's a dev-time tool, not a production integration. This will need real Gmail API access (OAuth2 + `google.golang.org/api/gmail/v1`, or similar), not an MCP call.
