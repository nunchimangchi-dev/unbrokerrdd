# Gemini pass 2: the dashboard still reads as a hacker terminal, not a trusted product

Read this in full before touching anything. Pass 1 (see `HANDOFF.md`,
"August 21, 2026: Visual Design Pass & Dashboard Redesign") made real,
verified improvements — the color-variable cruft is genuinely fixed, the
Three.js particle background and its CDN dependency are genuinely gone,
and the card grid is cleaner. Keep all of that. But the maintainer
actually ran the redesigned dashboard in a real browser (something pass 1
explicitly couldn't do) and it doesn't meet the brief: **it still reads as
a hacker/gaming-HUD terminal, just a tidier one.** This pass corrects that
specifically, with concrete evidence of what's still wrong — not a vague
"try again."

## What's actually still wrong — seen directly, not guessed

1. **The "AGENT TELEMETRY" sidebar panel** is a live-scrolling terminal log
   with `>` prompt markers and green/cyan monospace text
   (`> AWAITING_START...`, `> TARGETING BackgroundCheckme.org...`). This is
   the single most "hacker terminal" element on the page.
2. **The entire LOGS tab** is a full-page monospace console — timestamped
   lines in green/orange/red (`OPT_OUT_CONFIRMED`, `MANUAL_REVIEW`,
   `FAILED`). This is the strongest offender on the whole site. A log view
   can exist, but not styled as a movie-hacker console.
3. **A radar/sonar sweep visualization** in the sidebar ("BROKER STATUS
   LIVE FEED") — a circular scanning animation with dots. Decorative,
   reads as sci-fi surveillance tech, not a privacy tool.
4. **`.exe` branding throughout** — the browser tab title is literally
   `DATABROKER.exe`, and batch labels read `TRUTHFINDER_CASCADE.exe`. No
   real, trusted product (1Password, Proton, DeleteMe, Optery) brands
   itself as executable files. This needs to go entirely.
5. **`ALL_CAPS_UNDERSCORE` status jargon everywhere**: `AGENT_ACTIVE`,
   `PRIVACY_CORE: STABLE`, `OPT_OUT_CONFIRMED`, `MANUAL_REVIEW`,
   `DEMO_MODE: ACTIVE`. This is command-line/ops-center vocabulary, not
   how a consumer-facing (even if technical-audience) product talks to its
   user. Real status language: "Active," "Confirmed," "Needs review" —
   sentence case, no underscores, no scare-quotes-via-caps.

None of this resembles the references pass 1 cited (1Password, Proton) —
those products use calm sentence-case labels, no terminal jargon, no
`.exe` framing, no radar sweeps. The structural work (grid, color tokens,
no CDN dependency) is good. The *vocabulary and iconography* is still
fighting the brief.

## The actual goal, stated wholistically — hold this in mind throughout

This is a **privacy tool that handles real people's names, emails, and
opt-out requests against data brokers.** The entire point of the redesign
is that the interface should make someone trust it with that information —
calm, precise, competent. Terminal logs and hacker-HUD framing work
against that trust, no matter how polished the grid underneath is. Every
decision in this pass — icons, labels, copy, layout — should be evaluated
against that one question: *does this make someone more or less willing
to trust this tool with their personal data?* Keep that goal in mind as a
whole, not as a checklist of individual elements to swap — a genuinely
calm, trustworthy interface is a different *design language*, not the
same language with the loudest parts turned down.

## Research — and how to handle it when a search comes up empty

Search for real, current examples specifically from **privacy/security
products and their status/activity views** — how does 1Password show sync
status, how does Proton show account activity, how do DeleteMe/Optery show
opt-out progress across many targets (their actual dashboards, not
marketing pages). This is the same research direction as pass 1, but look
specifically at *status and log presentation* this time, since that's
where pass 1's hacker-terminal instinct came through strongest.

**If a search comes back empty, don't just reword it and don't silently
move on either.** Hold the actual goal above in mind and reason about
*why* that search might have failed relative to what you're actually
trying to learn — a different search wasn't finding "modern log UI," so
try a named real product instead of a generic category; a company's exact
dashboard isn't publicly documented, so look at their marketing screenshots
or a review site's walkthrough instead. Treat a failed search as a signal
to change *angle*, not just vocabulary. If you're still stuck after
actually reasoning about it that way, say so explicitly in `HANDOFF.md`
and explain why proceeding without that specific grounding is still
reasonable — never present the output as if the research happened when it
effectively didn't.

## Constraints — same hard boundaries as pass 1

- Cosmetic and structural-markup only. `internal/dashboard/static/index.html`
  and `aether-core.css` only — no `.go` files, no `databroker2.html`.
- Keep the no-build-step architecture and zero external dependencies (pass
  1's CDN removal was correct — don't reintroduce one).
- You have no browser and cannot visually verify your own work. Say so
  explicitly rather than claiming you saw it render — same as last time.
- **Work on the existing `design/dashboard-refresh` branch, not a new
  one** — this is a continuation of the same design effort, not a fresh
  pass. `git checkout design/dashboard-refresh` (it already exists with
  pass 1's commits on it).
- Commit locally. **Do not push, do not open a PR.**
- Append a new dated section to `HANDOFF.md` — don't rewrite pass 1's
  entry, add a new one for this pass specifically, including the explicit
  "could not visually verify" note again.

## Process

1. `git checkout design/dashboard-refresh` (already exists, don't branch again)
2. Re-read the current `internal/dashboard/static/index.html` as it
   stands now (post pass-1), specifically the sidebar telemetry panel, the
   LOGS tab, the radar sweep SVG/canvas, and every `.exe`/`ALL_CAPS`
   string literal in the markup and JS.
3. Research status/log presentation specifically from real privacy
   products, with the goal-informed zero-results reasoning above in mind.
4. Replace the five specific things listed above. Don't just soften them —
   redesign the log/status presentation and remove the radar sweep and
   `.exe` framing entirely, using calm sentence-case language throughout.
5. Commit locally on `design/dashboard-refresh`, don't push.
6. Append a new dated `HANDOFF.md` section: what changed this pass
   specifically, what you researched and cited, and the visual-verification
   caveat.
