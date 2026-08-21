# Gemini pass: real visual design for the unbrokerrdd dashboard

Read this in full before touching anything. This is the one thing you're
genuinely good at that the maintainer wants you doing — real visual craft,
not a token effort. Judged on whether it actually looks considered and
current, not on how many lines changed.

## Read the current state first — don't design blind

`internal/dashboard/static/index.html` (1787 lines, inline CSS/JS, no
build step) is the live dashboard. Read the whole `<style>` block and the
markup structure before changing anything. What's actually there right
now, so you're not rediscovering it the slow way:

- A deep obsidian/purple radial-gradient background (`#18094e` →
  `#0d0320` → `#060310`).
- A cyan/purple/pink "neon glow" palette — but look closely at the CSS
  custom properties: `--purple`, `--pink`, and `--blue` are all literally
  aliased to `var(--accent-cyan)`. Whatever multi-color system this was
  meant to be got collapsed to one color at some point, but the variable
  names never got cleaned up. That's real cruft worth fixing as part of
  building an actual coherent palette, not preserving.
- Heavy glow effects throughout: `box-shadow` with 12-28px blur radii in
  rgba-cyan/gold/red/green, applied liberally.
- A Three.js "purple pixel" particle background with fog, loaded from a
  public CDN (`cdn.jsdelivr.net`) — not vendored locally.
- Monospace (Space Mono) sci-fi-styled labels on SVG elements ("S1-2",
  "S3-4" etc).

## The actual brief

The maintainer's words: **"site should not look like AI glow, needs to be
modern and polished aesthetic, extremely visually appealing."** This
current look — dark obsidian, neon cyan/purple glow, floating 3D
particles, monospace sci-fi labels — is close to *the* recognizable
default "AI-generated cyberpunk dashboard" aesthetic at this point. Don't
just soften it; actually rethink it.

**There's a substantive reason this matters beyond taste, not just
aesthetics for their own sake:** this tool handles real personal data —
names, emails, opt-out requests against data brokers. A flashy
hacker-aesthetic dashboard is in tension with the trust this specific
product needs to project. A privacy tool that looks calm, precise, and
professional earns more confidence than one that looks like a video game
HUD. Let that inform the direction, not just "make it prettier."

## Research before designing — ground this in real references

Don't design from generic "modern dashboard" impressions. Search for real,
current (2026) examples specifically from **privacy/security-focused
products and data-management tools** — Little Snitch, 1Password, Have I
Been Pwned's newer UI, Proton's product family, DeleteMe/Optery's own
dashboards (the actual commercial competitors in this exact space), or
comparable security-tooling products with genuinely considered, current
design. Not generic SaaS dashboard mood boards — this specific category.

**If a search returns zero or empty results, do not silently continue as
if it succeeded.** Retry with a meaningfully different query, or stop and
say explicitly in the handoff notes that it failed and why proceeding
without that grounding is still reasonable. Cite what you actually looked
at — real product names — in your handoff notes, not vague impressions.

## The open question you need to actually decide, not default on

**What happens to the Three.js particle background is a real decision,
not a given.** It's a big part of what currently reads as "AI glow." You
could reimagine it into something restrained and purposeful (e.g. a subtle
network-graph visualization that actually represents the 85-broker sweep,
not decorative particles), or remove it entirely in favor of a calmer,
static design. Either is legitimate — but make the call deliberately and
say why in your handoff notes, don't just soften the existing effect by
default because removing a whole rendering system feels like a bigger
change. If you do keep something in that space, self-host it or inline it
rather than relying on the jsdelivr CDN — one less external dependency for
a security-sensitive tool that already avoids remote fonts/scripts.

## Constraints — hard boundaries

- **Cosmetic and structural-markup only.** No changes to the Go backend,
  the WebSocket protocol/message shapes, or any `.go` file. This is
  `internal/dashboard/static/index.html` and `aether-core.css`, nothing
  else.
- **`internal/dashboard/static/databroker2.html` is explicitly out of
  scope.** It's kept on purpose as a historical artifact per the README —
  don't touch it, don't "improve" it.
- **Keep the no-build-step architecture.** Plain HTML/CSS/JS, no framework,
  no bundler introduced. That's a deliberate existing choice, not an
  oversight to fix.
- **No new remote dependencies beyond what's already there**, and prefer
  reducing existing ones (the Three.js CDN load) over adding more.
- You have no browser and cannot visually verify your own work. Say so
  explicitly in your handoff notes rather than claiming you saw it render.
- Work on a fresh branch off `main` (`design/dashboard-refresh`). Commit
  locally. **Do not push, do not open a PR.** A human reviews live against
  the real running dashboard afterward.
- Before finishing: confirm the file still has valid HTML/CSS/JS by eye
  (no build step to catch errors for you here) — check your own markup and
  script blocks carefully since there's no compiler safety net.
- Append a dated section to `HANDOFF.md`: what changed, the real product
  references you actually researched, your reasoning on the Three.js
  question specifically, and the explicit "I could not visually verify
  this" note.

## Process

1. `git checkout main && git pull && git checkout -b design/dashboard-refresh`
2. Read `internal/dashboard/static/index.html` in full (style block +
   markup) before changing anything.
3. Research real references in the privacy/security-tooling space, with
   the zero-results rule in mind.
4. Decide and execute on the Three.js question explicitly.
5. Fix the aliased color-variable cruft (`--purple`/`--pink`/`--blue` all
   pointing at `--accent-cyan`) as part of building a real, coherent
   palette rather than leaving it standing.
6. Redesign for "modern, polished, trustworthy" — not just a lighter coat
   of the same look.
7. Commit locally, don't push.
8. Append to `HANDOFF.md`: what changed, references cited, the Three.js
   decision and reasoning, and the visual-verification caveat.
