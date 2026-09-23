# Presence checks — what they are and why the bar is this high

## The problem they solve

Every claim this project makes is a fraction. "8 of 95" is only meaningful if
the 95 is a list of places the subject is actually listed. It isn't. It's a list
of sites someone thought were worth targeting. Until each one is checked, the
denominator describes the tool's ambition, not anyone's real exposure — which
makes every coverage percentage built on it unearned.

A presence check answers one question per site: **does the subject appear here
at all?** It navigates, reads, and classifies. It never fills a form, never
submits anything, and never writes to the attempts log.

Three findings, and the asymmetry between them is the whole design:

| Finding | Meaning | Cost of being wrong |
|---|---|---|
| `present` | a matching result is listed | a wasted look |
| `absent` | the search ran and found nothing | **tells someone they are not listed when they are** |
| `undetermined` | the check could not be trusted | a manual look |

`absent` is the only finding that can do real harm, because it is a clean bill
of health. So it is the only one that has to be argued for. Every ambiguous
path — a challenge page, a login wall, an empty render, a missing API key, a
model that hedges — returns `undetermined`, which costs a manual look and
nothing else. This mirrors the same one-directional rule `checkDomTextFailure`
follows for submissions, for the same reason.

## Why a search URL must pass a control experiment

The first template ever tested here looked like it worked, and didn't.

`https://allpeople.biz/?ss={first+last}` was a reasonable guess from the site's
own search field name. The site ignored the parameter and served its browsable
A–Z directory instead. The classifier read a page full of other people's names,
found no match, and returned **absent** — with fluent, specific, entirely
wrong evidence. Every component behaved correctly and the result was a false
all-clear, which is the one answer this project must never produce.

A single search cannot tell "you are not listed here" apart from "this URL does
not search." Two can:

```
databrokergo presence --verify-template --url "<template>" --site "<name>"
```

It runs the same template twice — once with a name that must be found, once
with a name that cannot be — and only trusts it if the control comes back
`present` **and** the decoy comes back `absent`. Anything else is rejected,
with the reason.

Re-run against the template above, it now correctly refuses it.

**A template enters `agent.SearchTemplates` only after passing this.** Not from
the site's documentation, not from a pattern another broker on the same
platform uses. A guessed URL 404s, a 404 reads as "no results", and "no
results" classifies as absent — a wrong entry is worse than no entry, because
no entry is an honest "not checked".

## Verified templates

| Broker | Template | Verified | Verdict |
|---|---|---|---|
| allpeople | `https://allpeople.biz/?ss={first+last}` | 2026-09-23 | ✗ rejected — serves a directory index, does not search |

No template has passed verification yet. `SearchTemplates` is empty, and
`presence` reports every broker as "no verified search template" rather than
guessing. That is the honest state.

## The subject's name never reaches the terminal or the database

A presence check necessarily puts the real name into a URL and reads it back in
page text. None of that may escape:

- `RedactURL` masks query values and name-shaped path segments before display.
- `ScrubSubject` strips the name from classifier output **at the boundary where
  it enters the program**, so it reaches neither the terminal nor storage.

The second one is not cosmetic. Presence evidence is written to the `brokers`
table, and that database is synced to the production host and served behind
admin endpoints. Broker rows have always been free of personal data — that
property is exactly what made the sync safe to reason about. A classifier
helpfully quoting *"no results for &lt;real name&gt;"* would have silently ended
it. It did quote the name, once, during development. That is why the scrub sits
where it does and has a test.

## Reachability is a separate, cheaper question

Before asking whether someone is listed on a site, it is worth asking whether
the site exists.

```
databrokergo reach [--broker <id>] [--limit N] [--apply]
```

Two independent checks per domain: DNS resolution, then a real browser load. A
domain is only called dead when **both** fail, or when it resolves and serves a
parked/for-sale page. One signal is never enough — a resolver hiccup and a
genuinely dead zone look identical, and this project has already lost days to
classifications made from a single ambiguous signal. A domain that fails DNS
but loads in the browser is explicitly reported as local noise.

Without `--apply`, it writes nothing.
