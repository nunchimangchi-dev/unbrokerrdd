# Security & compliance baseline

Adapted from [skyrise's template](https://github.com/nunchimangchi-dev/skyrise/blob/main/docs/NEW-PROJECT-SECURITY-BASELINE.md).
This project's shape doesn't match a multi-tenant web app — it's a local,
single-user Go CLI + localhost dashboard, run by each user against their own
data. Several of the template's items genuinely don't apply here; several
others need reframing outward (toward the 85 external sites this tool
automates against) rather than inward (toward protecting a server from its
own users). Both directions are covered below, honestly.

## Audited 2026-08-21

Full pass: secrets/PII scan (patterns, test fixtures, kept mockup HTML,
CLAUDE.md, and the maintainer's actual name/email specifically), single
clean commit history (no old-history risk), build/vet/test verified
locally (not assumed), and a real dependency vulnerability scan.

**Clean:**
- No secrets, no real PII anywhere in the repo. `.env.example` uses
  obviously fake values; `.gitignore` correctly excludes `.env`,
  `secrets/`, `*.pem`/`*.key`, DB/state files, and local tool config.
- `go build ./...`, `go vet ./...`, and `go test ./...` all pass clean.
- `govulncheck ./...`: **0 exploitable vulnerabilities** — was 18 (all Go
  standard-library CVEs) before `go.mod`'s `go 1.26` directive was bumped
  to `1.26.7`, the latest patch release. Real fix, not a workaround: no
  breaking change was needed, unlike droppdd's equivalent finding.
- CI added (`build`, `vet`, `test`, `govulncheck`) — didn't exist before
  this audit.
- The dashboard binds to `127.0.0.1` only — correct, and already done.
- Redaction is real and tested (`TestConfig_RedactedNeverExposesSecrets`),
  not just claimed in a comment.
- **Rate limiting against the 85 external broker sites is real and
  correctly implemented** — a hard 5-second minimum between every broker
  dispatch (`internal/orchestrator/runner.go`, explicitly commented "never
  removed"), context-cancellation-aware, wired up from the CLI default.
  *Correction*: an earlier pass here claimed this was missing, based on a
  grep that searched for `time.Sleep`/`rate.Limit`/`backoff`/`Ticker` and
  missed the actual mechanism (`time.After` in a `select`). That was wrong
  and is corrected here rather than left standing.

**Fixed 2026-08-21 — model spend reduction:**
- **`checkDomTextFailure` added to `internal/agent/claude.go`** — a free,
  deterministic pre-check on the page's DOM text before ever calling Claude
  Haiku's vision API. Deliberately one-directional: it only short-circuits
  on a *failure* signal (explicit "invalid email", CAPTCHA text), never on
  success — a false "failed" just costs a manual re-check, but a false
  "success" would mean this tool reports a privacy removal that never
  happened, so potential successes always still go through real vision
  validation. Real tests added in `internal/agent/claude_test.go` (first
  coverage in that package).

**Real gaps found, not yet fixed:**
- **Test coverage still zero in `config`, `db`, `orchestrator`**
  (now has real coverage in `agent`, `strategies`, and — since the
  2026-08-22 admin panel work — `dashboard`).
- **No validation of the vision model's own output** for the *success*
  path specifically (see above — this is the same finding, narrowed: the
  dom-text pre-check only ever reduces failure-path calls, it doesn't
  address the deeper "what if Haiku hallucinates a success" question).
- **PII-handling discipline verified for the endpoints checked, not
  every code path.** As of 2026-09-17: the WebSocket broker-status feed
  carries no subject PII (broker id/name/status/URL/strategy only), the
  two endpoints that do return subject PII (`/api/profile`,
  `/api/users`) are gated to `role == "admin"` server-side, and
  `identityMiddleware` fails safe — an unrecognized visitor gets
  `"viewer"`, not `"admin"` (the one exception, standalone demo mode
  with no DB, only ever serves hardcoded fake `Jane Doe` data). Log
  output and error-path messages haven't been swept for incidental PII
  leakage, so this is "spot-checked," not "end-to-end."

## Audited 2026-09-17

Re-checked the items below after they were found stale in this file
relative to `HANDOFF.md`:
- **Broker-site ToS legality** was actually researched and documented on
  2026-08-21 (`HANDOFF.md`, `docs/TOS-LEGALITY-FINDINGS.md`) — the open
  items table below had it marked "Not started" by mistake; corrected.
- **`dashboard` test coverage** was added 2026-08-22 alongside the admin
  panel (`internal/dashboard/server_test.go`) — the table and the gap
  list above still said "zero"; corrected.
- Independently re-verified (not just re-read the doc) that `.env` and
  `databrokergo.db` have never been committed, across full git history,
  and that no real name/email/address appears anywhere in tracked files
  or commit messages.

## Items from the standard template that don't apply here, and why

- **Broken Auth/IDOR, File Uploads, Admin Panels** — no multi-user auth
  surface exists; the dashboard is a single-user, localhost-only panel by
  design, which already *is* the correct mitigation.
- **Staging & Environment Separation** — there's no deployed production to
  stage against; each user runs their own local instance. The closest
  equivalent already exists: the `--dry-run` CLI flag.
- **GDPR (as usually framed)** — no server holds any user's data under the
  maintainer's control; each user's data stays on their own machine. The
  real adjacent question is the ToS-legality point above, which isn't a
  GDPR question at all.

## Open items

| Item | Status |
|---|---|
| Secrets/PII scan | Clean, verified 2026-08-21 |
| Build/vet/test | Clean, verified locally |
| Dependency vulnerabilities | 0 exploitable, fixed via Go toolchain bump |
| CI | Added 2026-08-21 |
| Rate limiting against target sites | Done — verified real, was mistakenly flagged as missing earlier |
| Test coverage outside `strategies/` | `agent` (2026-08-21) and `dashboard` (2026-08-22) covered — `config`/`db`/`orchestrator` still not |
| Dom-text pre-check to reduce model spend | Done 2026-08-21 |
| Vision-model output validation | Not started |
| Broker ToS legality research | Done 2026-08-21 — see `HANDOFF.md` / `docs/TOS-LEGALITY-FINDINGS.md` |
| End-to-end PII-handling audit | Spot-checked 2026-09-17 (WS feed, `/api/profile`, `/api/users`, role fail-safe) — logging/error paths not yet swept |
