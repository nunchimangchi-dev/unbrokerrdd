# Gemini: build unbrokerrdd's admin panel - identity, roles, run control, PII

Read this in full before touching anything. This app currently has **no
concept of identity or roles at all** - the dashboard (`serve` command) is
a read-only viewer with zero authentication of its own. The only thing
gating access today is Cloudflare Access at the network edge (see
`docs/RUNBOOK-ADD-USER-CLOUDFLARE-ACCESS.md` in the sibling `skyrise`
repo, if reachable, for that setup - not required reading, just context).
This prompt builds the missing piece: real identity, a viewer/admin role
split, and the actual operator controls the dashboard has never had.

This is a bigger, more foundational change than droppdd's admin panel
(different app, different architecture - Go + SQLite + a single embedded
HTML/JS dashboard, no framework, no ORM). Do not port droppdd's
Next.js/Prisma patterns here - follow this codebase's existing
conventions, documented below.

## Why this shape specifically - read before objecting to any of it

This was scoped directly with the maintainer, not guessed:

- **No new login system.** unbrokerrdd is single-tenant (one opt-out
  campaign, one subject's PII, no per-user private data) - Cloudflare
  Access already authenticates every request that reaches this app at
  all. Building a second, redundant login (password, magic link, OAuth)
  would duplicate what Access already does. Instead: **trust the
  `Cf-Access-Authenticated-User-Email` header** that Access adds to every
  request it lets through. This is only safe to trust because the app is
  unreachable except through the tunnel Access sits in front of - it has
  no public port-forward and (per the deploy config) binds `0.0.0.0` only
  inside an isolated private network segment a public request can't reach
  directly. State this trust boundary explicitly in `HANDOFF.md` - don't
  silently assume the reader knows why trusting a header is safe here.
  Do not attempt to cryptographically verify the Access JWT
  (`Cf-Access-Jwt-Assertion`) - out of scope, the network-level trust
  boundary is the actual security boundary here, not the header itself.
- **Role table is additive to Access, not a replacement for it.** Access
  still decides who reaches the app at all (managed via Cloudflare's
  dashboard, per the runbook above - that doesn't change). The new
  `allowed_users` table this prompt adds decides, among people Access
  already let in, who gets `admin` vs `viewer` capabilities inside the
  app. If someone reaches the app via a valid Access session but has no
  row in `allowed_users`, treat them as `viewer` by default (not denied
  outright, not auto-admin) - Access already vouched for them, this table
  is scoped to admin-vs-viewer, not sign-in-at-all.
- **Secrets stay out of this entirely.** `ANTHROPIC_API_KEY` and
  `GMAIL_SENDER` must never be readable, writable, or displayed anywhere
  in the admin UI or its API - they stay exactly where they are today
  (env / macOS Keychain via `internal/config`). Only `SubjectName`,
  `SubjectEmail`, `SubjectState` move into the new admin-editable profile
  - nothing else from `Config`.

## Current state, exactly as it exists right now

- `internal/config/config.go`: `Config{SubjectName, SubjectEmail,
  SubjectState, AnthropicKey, GmailSender}`, loaded once at process start
  from env/.env/Keychain via `Load()`. `IsReady()` checks the first three
  plus `AnthropicKey` are non-empty before allowing a live (non-dry-run)
  batch. `Redacted()` exists for safe logging - reuse it, don't build a
  second redaction helper.
- `internal/db/store.go`: `New(path string)` opens SQLite and runs
  `CREATE TABLE IF NOT EXISTS` statements inline (see the `brokers` and
  `attempts` tables for the exact pattern) - **no separate migration
  files in this codebase, follow this same inline pattern** for any new
  tables.
- `internal/orchestrator/runner.go`: `Runner.RunBatch(ctx, BatchConfig{
  Strategy, DryRun, Limit, Delay})` is the actual batch-execution engine -
  it already enforces `cfg.IsReady()` before live runs, idempotency, and
  rate limiting. It takes a `notify func(StatusUpdate)` callback
  (`StatusUpdate{BrokerID, Status, DryRun, Notes}`) fired after each
  broker settles - **this is the integration point for live-updating the
  dashboard**, not a new mechanism.
- `internal/dashboard/server.go`: `Server.updateBroker(id, status)` is
  the existing hook that updates in-memory state and broadcasts to every
  connected WebSocket client via `s.hub.broadcast` - reuse this exact
  method as (or as the model for) the `notify` callback passed into a
  web-triggered `Runner`, so a run started from the browser streams live
  into the same dashboard UI that already exists, not a separate view.
  `ServeHTTP` currently only routes `/` (embedded static files) and
  `/ws` - all new admin routes get added to that same mux.
  **There's a stale comment above `Serve()`**: "Security: addr MUST be a
  loopback address... Never bind to 0.0.0.0 — this dashboard has no auth
  layer." This is now inaccurate (the deploy config already binds
  `0.0.0.0` behind Cloudflare Access, and this prompt adds a real
  auth/role layer) - update that comment to describe the actual current
  security model instead of leaving a contradictory warning in the code.
- `cmd/databrokergo/main.go`: the CLI has `run`, `status`, and `reset`
  subcommands with real functionality that **the web dashboard has no
  equivalent of today** - `run --strategy N [--dry-run] [--limit N]` and
  `reset --broker <id>` are SSH-only right now. This prompt's job is to
  expose equivalents of these two specifically through the web UI,
  admin-gated.
- Visual system: the dashboard's existing sage/forest-green palette (see
  `internal/dashboard/static/index.html`'s `:root` CSS custom properties)
  and the recently-added `.exposure-wrap`/circular-progress-ring pattern
  are the established look - match it for any new admin UI, don't
  introduce a new visual language.

## What to build

### 1. Identity middleware + role table (foundation - build this first)

- New table `allowed_users` (email TEXT PRIMARY KEY, role TEXT NOT NULL
  CHECK(role IN ('viewer','admin'))), created inline in `internal/db`
  alongside the existing tables.
- Middleware (wrapping `ServeHTTP` or added into the mux setup) that
  reads `Cf-Access-Authenticated-User-Email` from the incoming request,
  looks up (or defaults, per the rule above) a role, and makes it
  available to handlers - a request context value is the natural fit
  here.
- **Bootstrap problem to solve explicitly**: on first boot, `allowed_users`
  is empty, so nobody is admin and nobody can ever become admin through
  the UI - a real lockout. Solve it the same way `config.Load()` already
  reads from env: an `UNBROKERRDD_ADMIN_EMAIL` env var that, if set and
  `allowed_users` is empty, seeds that one row as `admin` on startup.
  Idempotent - don't re-seed or overwrite if the table already has rows.
- Every new admin route/endpoint below must check for `admin` role
  independently, not just rely on a shared middleware existing somewhere
  else - same defense-in-depth expectation as droppdd's admin panel
  (every mutating handler re-checks, not just the router).

### 2. Subject PII profile (replaces .env for these three fields)

- New table `subject_profile` (single row - id INTEGER PRIMARY KEY CHECK
  (id = 1), name TEXT, email TEXT, state TEXT) - or a key-value table if
  that fits this codebase's style better, your call, but keep it to
  exactly these three fields, nothing else.
- Admin-only view + form to see and edit these three fields, replacing
  the need to SSH in and edit `.env` + restart the process.
- `config.Load()` (or a new sibling function) needs to read these three
  fields from the DB instead of/in addition to env - decide and document
  in `HANDOFF.md` which takes precedence if both exist (recommend: DB
  wins if a row exists, env remains the fallback/bootstrap path for a
  fresh install with no DB row yet).

### 3. Run control (the highest-value piece - a real web equivalent of `run`)

- Admin-only endpoint to start a batch: strategy (1-6), dry-run toggle,
  limit. Construct a `Runner` via `orchestrator.New(store, cfg, agents,
  notify)` exactly as `main.go`'s `run` case already does, with `notify`
  wired to update the dashboard live (reuse `s.updateBroker`, per above).
- Run it in a goroutine, not blocking the HTTP response - the triggering
  request should return immediately once the batch starts, with results
  streaming through the existing WebSocket mechanism.
- Guard against starting a second batch while one is already running
  (a simple in-memory "is a batch currently running" flag on `Server` is
  enough - reject a second start attempt with a clear error rather than
  letting two batches race against the same store).

### 4. Per-broker retry

- Admin-only endpoint wrapping `store.Reset(id)` (already exists, same
  as the CLI's `reset --broker <id>`) - resets one broker back to
  pending. Should update the in-memory/broadcast state too (same
  `updateBroker` hook) so the UI reflects it immediately.

### 5. Access management (who has `admin` vs `viewer`)

- Admin-only view of everyone currently in `allowed_users`, with the
  ability to add a row (email + role), change someone's role, or remove
  a row. Same last-admin invariant as droppdd's admin panel: **never
  allow the last `admin` row to be demoted or removed** - block it with
  a clear error, don't let it happen silently.
- This does **not** touch Cloudflare Access's own allowlist (who can
  reach the app at all) - that stays exactly as documented in the
  runbook. This is purely the in-app role table.

## Explicit non-goals

- No changes to `strategies/`, `agent/`, or the actual opt-out automation
  logic itself - this prompt is entry points and control surface only.
- No cryptographic verification of the Access JWT - documented tradeoff
  above, don't add it.
- No exposure of `AnthropicKey`/`GmailSender` anywhere in the new UI or
  API responses, including error messages or logs.
- Don't touch the existing public (non-admin) dashboard view's visual
  design - new admin UI is additive.

## Verification

You have no browser and cannot visually verify - say so explicitly in
`HANDOFF.md`, same as every prior prompt. This one is genuinely
access-control-critical in a new way for this codebase (it's the first
real identity/authorization layer this app has ever had), so write a
specific manual test plan covering: (a) a request with no
`Cf-Access-Authenticated-User-Email` header (or an email not in
`allowed_users`, defaulting to viewer) cannot reach any admin
endpoint/route; (b) the last-admin invariant actually blocks removal/
demotion; (c) starting a second batch run while one is active is
rejected, not silently queued or racing; (d) confirm nothing in
`internal/config` or the new profile/role code ever logs or returns
`AnthropicKey`/`GmailSender`.

## Constraints

- New branch: `git checkout -b feature/admin-panel`.
- Keep the no-build-step architecture and zero external dependencies for
  the frontend (embedded static HTML/JS/CSS, same as today) - the
  backend can obviously use Go stdlib + whatever's already imported
  (`gorilla/websocket` is already a dependency).
- Commit locally. **Do not push, do not open a PR.**
- Append a dated `HANDOFF.md` section: what was built, the trust-boundary
  reasoning restated in your own words (confirm you actually understood
  it, don't just copy this prompt), the manual test plan, and the
  visual-verification caveat.

## Process

1. `git checkout -b feature/admin-panel`
2. Read `internal/config/config.go`, `internal/db/store.go`,
   `internal/orchestrator/runner.go`, `internal/dashboard/server.go`, and
   `cmd/databrokergo/main.go` in full before changing anything.
3. Build the identity middleware + `allowed_users` table + bootstrap
   (section 1) first - everything else depends on it.
4. Build sections 2-5 in whatever order makes sense given what you find,
   but run control (section 3) is the highest-priority piece if you have
   to make tradeoffs on depth vs. breadth under time pressure.
5. Update the stale `Serve()` security comment.
6. Commit locally on `feature/admin-panel`, don't push.
7. Append the dated `HANDOFF.md` section with the manual test plan.
