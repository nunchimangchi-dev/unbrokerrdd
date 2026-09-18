# databrokergo

**Purpose:** Fully automated AI agent system for removing personal data from ~85 data brokers. Built as a real-world showcase of AI solving a genuine privacy problem — not just a script. Every meaningful decision is made by an AI model.

**Owner:** you@example.com

---

## What this is

An AI-powered data broker opt-out orchestrator. It:
1. Loops through a queue of data brokers (SQLite-backed)
2. Dispatches work to specialized agents based on the broker's strategy type
3. Updates a real-time dashboard (3D pixel art, localhost:8080) via WebSocket
4. Runs incrementally — each batch is QA-gated before the next one starts

All 85 brokers and their strategy assignments live inline in `internal/dashboard/brokers.go` — that's the single source of truth (no separate registry file).

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
    brokers.go    ← all 85 brokers + strategy assignments (source of truth)
    server.go     ← Go HTTP + WebSocket server on :8080
    static/       ← frontend: dashboard UI (index.html + aether-core.css)
strategies/
  strategy1_truthfinder.go      ← 7 sites, 1 form submission — IMPLEMENTED
  (strategies 2-6: planned, not yet built — see below)
```

Note: earlier drafts of this doc described a broader file layout (`agent/browser.go`, `agent/email.go`, `agent/whois.go`, a `databrokergo.md` registry file) that was never actually built — this section now reflects what's really on disk.

---

## The 6 Strategies

| # | Name | Sites | Mechanism | Status |
|---|------|-------|-----------|--------|
| 1 | TruthFinder Affiliates | 7 | 1 suppression request at suppression.peopleconnect.us (PeopleConnect's shared portal), chromedp + Claude Haiku vision validation | **Built** — corrected 2026-09-17, see HANDOFF.md |
| 2 | Hidden Opt-Out Pages | 15 (was 14 — added AdvancedBackgroundChecks 2026-09-18) | Navigate to /opt-out URL, fill form | **Partially built** — 2/15 (CheckPeople, AdvancedBackgroundChecks), rest route to manual |
| 3 | Privacy Page Discovery | 25 | Scrape contact email → send CCPA email | Planned |
| 4 | Business Directories | 18 | Search name → skip if not found | Planned |
| 5 | Phone Directories (WHOIS) | 12 | WHOIS lookup → send CCPA email | Planned |
| 6 | Profile Brokers | 9 | Account-based deletion or flag manual | Planned |

The broker registry (86 targets in `brokers.go`, was 85 before AdvancedBackgroundChecks was added 2026-09-18) is fully seeded across all 6 strategy buckets today. Strategy 1's agent is wired up for its whole bucket (one PeopleConnect suppression submission cascades all 7 — see the 2026-09-17 correction in `HANDOFF.md`, it used to hit the wrong TruthFinder control entirely). Strategy 2 (`strategies/strategy2_checkpeople.go` + `strategy2_advancedbackgroundchecks.go`) is a router, not a single mechanism like Strategy 1 — each broker.ID needs its own verified selectors (real sites have cookie banners, iframes, bot-check interstitials, and multi-step disclosure that don't yield to one generic vision pass), so only `checkpeople` and `advancedbackgroundchecks` have working handlers; every other Strategy 2 broker.ID is deliberately routed to `StatusManual` rather than guessed at. See the 2026-09-17 and 2026-09-18 entries in `HANDOFF.md` for what was tried, what actually blocks automation on the rest of the BADBOOL list (bot-checks that block chromedp outright, profile-URL disambiguation needed for correctness, PII fields the tool deliberately doesn't collect), and why. Strategies 3-6 remain fully roadmap, not yet implemented.

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
skipped     → business directory with no personal results
manual      → requires human action (flagged by agent)
```

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
go run ./cmd/databrokergo serve        # start dashboard + agent loop
go run ./cmd/databrokergo run --batch 1 # run specific batch
go run ./cmd/databrokergo status        # print current state table
go run ./cmd/databrokergo reset --broker "TruthFinder" # reset a broker to pending
```

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
