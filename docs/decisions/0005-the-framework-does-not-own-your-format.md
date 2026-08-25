# 0005 — The framework does not own your format

Status: **accepted and built** · 2026-08-19 · Replaces the README's §3 birth rule and
supersedes the gate half of [`0003`](./0003-briefs-have-a-type.md) ·
Implemented by [`modules/threads.md`](../modules/threads.md),
[`modules/artifacts.md`](../modules/artifacts.md) and
[`modules/mcp.md`](../modules/mcp.md)

## Context

The birth rule was the framework's proudest constraint: no thread entered
implementation without a Brief carrying a `## Definition of Done` written as
checkboxes, plus a seeded Logbook, unless the caller declared the thread small. The
closing counterpart refused `complete_thread` while any DoD item was unticked. The
linter refused a page with two H1s, a Logbook with no todo, a Brief with no DoD.

Months of using the tool on real work produced a consistent complaint: **the gate is
severer than the problem it solves.** Three things kept happening.

1. **Everybody writes their work down differently.** A debugging thread is a
   symptom and a trail; a design thread is a sketch and three open questions; a
   chore is one line. Forcing all of them through Context / Outcome / Definition of
   Done produced sections written to satisfy the parser — headings with a sentence
   under them that nobody would have written unprompted.
2. **The gate rejected work that already existed.** Pasting a document somebody had
   written elsewhere — the honest starting point for most threads — meant editing it
   into our shape first. That is a tax on the exact act the tool exists to
   encourage.
3. **A refused call moves the work somewhere unmeasured.** This is the part that
   actually costs something. When `birth_thread` refuses, the work does not become
   better-defined; it goes into a chat message, a scratch file, a note app. The
   framework then measures nothing, which is worse than measuring a scruffy
   document.

The rule was defensible on its own terms. A thread nobody has thought about is a
thread nobody finishes — that is true. But *thinking about it* and *writing it in our
five headings* are not the same act, and only the second one was ever enforceable.

## Decision

**A thread requires a name. Nothing else.**

The tool provides an automated framework — bubbles, threads, and a temperature
derived from evidence. Use it however you want; your ideas and your tasks are warmed
or cooled by Bubble Work's policy, not by their shape.

### What is no longer enforced

| Was | Is now |
|---|---|
| `birth_thread` refuses without a Brief | `create_thread` needs `name`; `body` is free-form markdown, stored verbatim |
| the Brief must contain `## Definition of Done` with `- [ ]` items | offered, and reported on when absent — never refused |
| a Logbook is required unless `small_thread: true` | no shape is required, so nothing needs excusing; `small_thread` is accepted and ignored |
| `complete_thread` refused while DoD items were unticked | never refused; the answer carries `unmet_dod` |
| the linter refused two H1s, a Logbook with no todo, a Brief with no DoD | all warn |

### What is still refused

One thing: **a duplicated reserved heading** (`## Brief`, `## Logbook`,
`## Definition of Done` appearing twice on a page). That is machinery, not taste — the
first section's range stops at the second heading, so the canonical region becomes
empty and every region-scoped write addresses nothing. A page that breaks its own
addressing is not a stylistic choice.

### What replaces the gate: say it, do not refuse it

Everything the gate used to demand is now *observed*.

- `create_thread` returns a `message` naming what it noticed — no document, no finish
  line, nothing tickable.
- `audit_bubble` reports a `missing` line per thread: a document, a finish line,
  something to tick, a next action. It is an observation across a whole bubble at
  once, which is the view a person cannot assemble by reading threads one at a time.
- `complete_thread` reports `unmet_dod`, so finishing over an open item is a visible
  decision rather than an invisible one.

### Two headings keep their machinery, as an offer

`## Definition of Done` and `## Logbook` still mean something to the tool — the first
is reported at completion and tracked by the audit, the second is its own addressable
region. They are advertised as *what you get if you use them*, never as what you owe.
The typed Brief templates from [`0003`](./0003-briefs-have-a-type.md) survive on the
same footing: a scaffold the CLI, the web form and the MCP tool can offer.

### Checkboxes count anywhere

Consequence of the same principle: if the format is yours, the plan may live under
your headings. `CountDone` now counts ticked boxes across the whole document, and
`toggle_todo` addresses items in the `document` region. A thread whose plan is a
checklist under `## Pasos` produces exactly the evidence a `## Logbook` does.

In the document, only real `- [ ]` boxes are addressable. The Logbook's
bullet-as-todo fallback would read a paragraph of prose bullets as a task list —
harmless inside a section that claims to be a plan, wrong everywhere else.

## Alternatives rejected

| Alternative | Why not |
|---|---|
| Keep the gate, add a `--force` flag | It already had one (`small_thread`, `force`). An escape hatch that everybody uses is not a gate; it is a speed bump plus a guilt tax. |
| Keep the gate for `feature`/`bug`, drop it for `chore` | Predicts which work deserves rigour from a dropdown chosen before the work starts. That prediction is wrong often enough to be worse than no prediction. |
| Refuse at completion but not at creation | Moves the wall to the least useful moment: after the work is done, when the checklist is most likely to be stale and least likely to be re-litigated honestly. |
| Drop the Definition of Done concept entirely | Overcorrects. The DoD earns its keep for people who want it — it is the only thing that makes "is this finished?" answerable by somebody else — and it costs nothing when absent. |
| Keep the linter refusing two H1s | Two H1s are invisible to the table of contents. Worth saying; not worth losing a write over, and never worth losing the *document* over. |

## Consequences

**Heat is unchanged and now reaches further.** Any edit to the document already
counted ([`0004`](./0004-writing-is-the-work.md)); with checkboxes counted everywhere,
a plan in somebody's own shape produces the same `completed-todo` evidence as one in a
Logbook. Nothing about the temperature model moved.

**`birth_thread` stays as an alias of `create_thread`**, same arguments, same
behaviour. Every agent, script and transcript in existence says `birth_thread`, and
renaming a verb is not worth breaking them. The CLI likewise: `bubble new`, with
`bubble birth` still accepted.

**"Birth" survives as a word for the event, not for a gate.** A created thread is
still production and still hot from its first moment
([`0002`](./0002-birth-is-production.md)) — that decision stands, with its condition
relaxed from *two artifacts exist* to *somebody defined a piece of work*.

**`audit_bubble`'s `needs_repair` is now an advisory count.** The field name stays for
compatibility; what it counts is threads with an observation, not threads in
violation.

**A messier corpus is the accepted price.** Some threads will be a title and two
sentences. That was already true — they were just being written somewhere the tool
could not see them.
