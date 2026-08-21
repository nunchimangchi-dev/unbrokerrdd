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
- **Test coverage still zero in `config`, `dashboard`, `db`, `orchestrator`**
  (now has real coverage in `agent` and `strategies`).
- **No validation of the vision model's own output** for the *success*
  path specifically (see above — this is the same finding, narrowed: the
  dom-text pre-check only ever reduces failure-path calls, it doesn't
  address the deeper "what if Haiku hallucinates a success" question).
- **Broker-site Terms of Service legality is an open, undocumented
  question.** Distinct from any of the 14 points as originally framed —
  closer to CFAA-adjacent territory than GDPR/privacy. Whether automating
  opt-out submissions against a given site's own ToS is legally fine
  hasn't been researched or documented anywhere in this repo.
- **PII-handling discipline verified for one config object, not
  end-to-end.** The redaction test proves `Config.Redacted()` works; it
  doesn't prove the dashboard's HTTP/WebSocket responses or any log output
  never leak the subject's name/email during normal operation.

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
| Test coverage outside `strategies/` | Started 2026-08-21 — `agent` now covered, `config`/`dashboard`/`db`/`orchestrator` still not |
| Dom-text pre-check to reduce model spend | Done 2026-08-21 |
| Vision-model output validation | Not started |
| Broker ToS legality research | Not started |
| End-to-end PII-handling audit | Not started (only the config-object path verified) |
