# Gemini pass: research broker-site ToS legality of automated opt-outs

Read this in full before starting. This is a research and documentation
task — a findings file, not code. Get the legal/factual claims wrong and
it's worse than not researching at all, so cite real sources for everything,
not general impressions.

## What this pass is and isn't

- **Output**: a single new file, `docs/TOS-LEGALITY-FINDINGS.md`. No code
  changes, no new dependencies, no `.ts`/`.go` files touched.
- Work on a fresh branch off `main` (`docs/tos-research`). Commit locally.
  **Do not push, do not open a PR.**
- This informs a real decision the maintainer is making about running this
  tool against real accounts — get it right, and say plainly where you're
  uncertain rather than smoothing over gaps.
- Append a dated section to `HANDOFF.md`: what you found, what you
  couldn't determine, and every source you actually used.

## The question this needs to answer

Does automating opt-out submissions against a data broker's own site risk
violating that site's Terms of Service in a way that carries real legal
exposure — for an individual acting on their own behalf, using their own
identity, submitting their own opt-out request?

This is **not** a GDPR or privacy-law question — it's closer to
CFAA/unauthorized-access-adjacent territory (does automated form
submission on a site whose ToS restricts "bots" or "automated access"
create legal risk distinct from just filling out the same form by hand).

## Research, prioritized

1. **CCPA's authorized-agent provision, specifically.** California law
   (Cal. Civ. Code § 1798.135 and related CCPA regulations) explicitly
   contemplates consumers using an authorized agent to submit opt-out/
   deletion requests on their behalf. Research exactly what this provision
   requires and permits, and whether it provides real legal footing for a
   tool like this one — this could be a genuinely favorable finding, not
   just a risk to flag. Don't assume the answer either direction; find the
   actual statutory language and any CPPA (California Privacy Protection
   Agency) guidance on it.
2. **California's Delete Act (SB 362) / the DROP platform at
   privacy.ca.gov.** Research its actual current rollout status and
   timeline as of now, not an assumed one — the maintainer's own
   understanding is that it exists but has a long lag before anything is
   actually actioned by the state. Confirm or correct that, with sources.
   This matters for how the tool's own README should honestly position
   itself relative to the state mechanism (a faster, narrower-immediate
   alternative — not a replacement, and not competing with a state process
   that already covers the same ground well).
3. **Precedent from comparable, already-operating tools.** Services like
   DeleteMe, Optery, Incogni, and Kanary already do automated (or
   semi-automated) opt-out submission at scale, commercially, across many
   of the same brokers. Research whether any of them have faced legal
   action specifically over ToS/automation conflicts, and how they
   publicly describe their legal basis for operating. This is real
   precedent, not speculation — cite what you actually find.
4. **TruthFinder's specific Terms of Service** — this is the one broker
   with an actual implemented strategy today (`strategies/strategy1_truthfinder.go`).
   Read its live ToS directly and note anything relevant to automated
   access, bots, or opt-out submission methods specifically.
5. **General industry/legal landscape** for automated opt-out tooling —
   is this a well-established, normalized practice at this point, or a
   genuinely contested one? Search broadly enough to have a real, current
   answer, not an assumption carried over from older automation-lawsuit
   history (e.g. hiQ v. LinkedIn) that may not be the current state of law
   on this specific question.

**If a search returns zero or empty results, do not silently continue as
if it succeeded.** Retry with a meaningfully different query, or stop and
say explicitly in `HANDOFF.md` that it failed and why proceeding without
that grounding is still reasonable. Never write findings as if research
happened when it didn't.

## What good output looks like

`docs/TOS-LEGALITY-FINDINGS.md` should have:
- A clear, direct answer (or honest "genuinely unclear, here's why") to the
  core question above, for the specific case of an individual using this
  tool on their own behalf.
- A section specifically on the CCPA authorized-agent angle, since that's
  the most likely source of real legal footing, not just risk-avoidance.
- Real citations — statute sections, actual URLs, actual company names —
  not vague "some sources suggest" language.
- An explicit, separate note flagging that the calculus is materially
  different if this tool is ever operated as a hosted service on behalf of
  many other people, rather than run locally by one person for themselves
  — don't conflate the two scenarios in the analysis.
- Nothing here substitutes for actual legal advice — say that plainly at
  the top, same as the legal-draft pass's disclaimer pattern.

## Process

1. `git checkout main && git pull && git checkout -b docs/tos-research`
2. Read `strategies/strategy1_truthfinder.go` and `README.md` first, for
   context on what's actually implemented today.
3. Research in the priority order above, with the zero-results rule in
   mind throughout.
4. Write `docs/TOS-LEGALITY-FINDINGS.md`.
5. Commit locally, don't push.
6. Append to `HANDOFF.md`: findings summary, sources used, and any open
   question you couldn't resolve.
