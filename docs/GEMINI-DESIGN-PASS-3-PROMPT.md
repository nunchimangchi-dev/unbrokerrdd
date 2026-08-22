# Gemini pass 3: replace the color system with a concrete target palette

Read this in full before touching anything. Passes 1 and 2 fixed the
structural/vocabulary problems (dead code, `.exe` branding, terminal-log
styling, jargon) — see `HANDOFF.md` for both. What's left is narrower and
concrete: **the color system itself is a generic blue→indigo→purple→pink
AI-SaaS gradient**, and it needs to become something else specific, not
another round of "make it feel more premium."

## The current palette, exactly as it exists right now

```css
--bg-primary:      #0b0f19;  /* near-black navy */
--accent-light:    #60a5fa;
--accent-gradient: linear-gradient(135deg, #3b82f6 0%, #6366f1 100%);
--accent-purple:   #8b5cf6;
--accent-pink:     #ec4899;

--s1: #3b82f6;  /* Strategy 1: blue */
--s2: #6366f1;  /* Strategy 2: indigo */
--s3: #8b5cf6;  /* Strategy 3: purple */
--s4: #10b981;  /* Strategy 4: emerald */
--s5: #f59e0b;  /* Strategy 5: amber */
--s6: #f97316;  /* Strategy 6: orange */
```

That blue→indigo→purple→pink spread across the base accent and the first
three strategy colors *is* the generic AI-product palette at this point —
recognizable the same way the dark-obsidian-neon-glow look was in pass 1.
Rainbow-across-strategies also isn't actually meaningful — six unrelated
hues don't read as a coherent system, just "each one got a different
crayon."

## The target: a specific, real reference — not "something calm"

The direction to build toward: **sage/olive green as the primary accent,
dark forest green for contrast/dark sections, warm cream/off-white for
light surfaces.** This isn't an abstract mood — it's grounded in two real,
current references the maintainer picked out directly:

- **FinanceUs (Dribbble, JA. Parvez for Wingly)** — a SaaS dashboard
  (not just a brand page) using exactly this palette: soft sage as the
  interactive/accent color, dark forest green for section backgrounds and
  a circular gauge component, warm cream elsewhere. The designer's own
  description: *"Soft sage palette + serif typography + dark forest green
  sections — calm, credible, and premium."* This is the primary reference
  because it's an actual dashboard with cards, a gauge, and data display —
  directly comparable to what unbrokerrdd needs, not just a logo showcase.
- **Webrij (Dribbble, SimonfelDesign)** — a brand-identity system in the
  same family (deep near-black-green, sage/olive as the accent, cream for
  light sections), useful for how the palette holds up across dark and
  light contexts specifically.

Both are worth a real look (search for them, or reason from the
descriptions above if search doesn't surface the actual pages) before
building the new tokens — don't just take the hex-adjacent guess, look at
how the palette is actually *used*: what's accent vs. background vs. text,
how dark and light sections relate to each other.

## One thing to actually think about, not just copy

Both references are fintech/general-SaaS, not privacy/security tooling.
Before committing to a straight port, spend a little real thought on
whether sage/forest-green needs any adaptation for a *privacy* product
specifically (vs. a finance product) — or whether the "calm, grounded,
credible" quality that makes it work for finance is exactly the same
quality that makes it right here too. Say what you concluded and why in
`HANDOFF.md`, don't just silently apply it.

## What actually needs to change

- Replace `--accent-purple`, `--accent-pink`, and the blue→indigo
  `--accent-gradient` — none of these survive into the new palette.
- Rebuild `--s1` through `--s6` as a coherent family within the new
  palette (tonal variations of sage/forest-green, or a small set of
  muted, deliberately-chosen complements) rather than six unrelated hues.
  They still need to be visually distinguishable from each other at a
  glance — don't sacrifice function for restraint.
- `--bg-primary` and the dark surface tokens should move toward the deep
  forest-green territory rather than navy, if that's what the references
  actually show — verify this against what you find, don't assume.
- Status colors (success/failed/pending/manual) can reasonably stay closer
  to their current hues (green-success, red-failed are fairly universal),
  but reconsider them against the new background so they still have real
  contrast and don't clash with the new accent family.

## Research — and the same zero-results discipline as pass 2

Hold the actual goal in mind throughout: this is a privacy tool, and the
palette should read as calm and trustworthy, the same underlying goal as
passes 1 and 2, now narrowed to color specifically. If a search for either
reference or for additional grounding comes back empty, don't reword it
and don't silently move on — reason about *why*, relative to that goal,
and pick a genuinely different angle (a different search for the same
designer's other work, a broader "sage green SaaS dashboard" category
search, etc.). If still stuck after actually reasoning through it, say so
explicitly in `HANDOFF.md` and explain why proceeding without that
specific grounding is still reasonable.

## Constraints — same as passes 1 and 2

- Cosmetic and structural-markup only. `internal/dashboard/static/index.html`
  and `aether-core.css` only.
- Keep the no-build-step architecture, zero external dependencies.
- You have no browser and cannot visually verify your own work — say so
  explicitly, same as before.
- **Continue on the existing `design/dashboard-refresh` branch.**
  `git checkout design/dashboard-refresh` — don't create a new one, this
  is the same design effort.
- Commit locally. **Do not push, do not open a PR.**
- Append a new dated `HANDOFF.md` section for this pass specifically —
  don't rewrite the prior two entries.

## Process

1. `git checkout design/dashboard-refresh`
2. Read the current color token block in `internal/dashboard/static/index.html`
   in full (the CSS custom properties near the top) before changing
   anything.
3. Research the two named references, with the goal-informed zero-results
   reasoning above in mind if searches come up short.
4. Think through the fintech-vs-privacy adaptation question and decide
   deliberately.
5. Replace the color tokens per "what actually needs to change" above,
   then sweep the file for any hardcoded hex values that duplicate what's
   now a token (pass 1 left some of these) and point them at the tokens
   instead.
6. Commit locally on `design/dashboard-refresh`, don't push.
7. Append a new dated `HANDOFF.md` section: the palette decisions made,
   the fintech-vs-privacy reasoning, what was researched and cited, and
   the visual-verification caveat.
