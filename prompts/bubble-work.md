# Bubble Work

A place to plan work, write it down, and see what has gone cold.

## The model

- **Workspace** — the boundary for a body of work. Everything below belongs to one.
- **Bubble** — a durable grouping of related work; the unit of *attention*. It has
  an outcome and can die.
- **Thread** — one executable unit of work. It carries a markdown document.
- **Heat** — evidence that *reality changed*. Not activity.

## What warms, and what does not

| | |
|---|---|
| **any** write to a thread's document | warms |
| creating a thread | warms — defining a piece of work is production |
| completing a thread | warms |
| adding a link to external evidence | warms |
| commenting | **no** — it is pulse: it keeps a thread out of the grave, never wakes it |
| renaming, labelling, changing state, assigning | **no** |
| writing in `docs/` | **no** — recorded, never warms |
| a write that leaves the file byte-identical | **nothing is recorded** |

Do not manufacture activity to keep something warm. It does not work — the rules
above are the whole of it — and a bubble that has gone quiet is information, not a
failure. Revive it with real work, redefine it, or close it.

## Where files live

```
<workspace>/
  README.md              the workspace's front page
  threads/               one file per thread
    14-portar-motor.md     THE thread's document
    14-portar-motor/       anything else that thread writes, beside it
      notas.md
  docs/                  the wiki: guides, references, notes. Nests freely.
  assets/                images, flat, referenced from anywhere as assets/<name>
```

`threads/` nests exactly one level: a thread's own document is the file, and
whatever else that thread needs — a research note, a design, a log that does not
belong on the main page — lives in the folder beside it. Writing there is the
THREAD's writing: it warms that thread, not the workspace.

Only `.md` and `.excalidraw` are writable, and only images live in `assets/`.
Anything outside this shape is refused.

A thread's file is named by the server and moves when the thread is renamed — its
folder moves with it. Do not depend on a path staying still; a thread id does.

## Writing

Every change goes through one call, in three shapes. Send **exactly one** of them:

```jsonc
{"base": "<hash>", "content": "…"}                      // replace the whole file
{"base": "<hash>", "edits": [{"old": "…", "new": "…"}]} // surgical, by quoting
{"base": "<hash>", "todo": {"index": 0, "done": true}}  // tick a checkbox
```

**`base` is the hash you got when you read the document.** It is required. If the
file changed since you read it, the write is refused and the error carries the hash
that is on disk now.

When that happens: **read it again and re-apply your change to the new text.** Never
retry with the same base, and never fall back to sending the whole `content` — the
version you hold is missing whatever the other writer just added.

Prefer `edits` over `content`. Replacing a whole file to change one line is how a
paragraph you never read gets destroyed.

Two errors you will meet, and both are telling you something useful:

- *that text is not in this section* — you quoted something that is not there.
  Read the document again; do not guess.
- *that text appears more than once — quote more of it* — your `old` is ambiguous.
  Include a surrounding line so it matches once. Passing `all` replaces every
  occurrence, and only do that when you mean every one.

Checkboxes are counted anywhere in the document. Only real `- [ ]` boxes count; a
plain bullet is a sentence.

## Before you start, and what else you can say

**Read the timeline.** Heat tells you a thread is warm; `timeline` tells you WHAT
made it warm and when. Starting without it is how you redo something that was
finished on Tuesday.

Everything about a thread that is not its document goes through `set_thread`: the
bubble that carries it, the objective it is FOR, when it is due, its impact and
urgency. **Priority is not among them** — the server derives it from impact ×
urgency, and there is no second way to write it.

A bubble is created with an outcome and can be closed with a sentence
(`set_bubble`). Closing is a decision with a date, not a delete: it says the work
no longer competes for attention, and reopening is one call. Say who is
accountable when you know — a bubble that has gone quiet with nobody accountable
is a grave, not a nap, and that is the band it will be shown in.

**When something is raised and is not yet work, `capture` it.** A note has no
outcome and no evidence; turning every remark into a thread is how a backlog
fills with rows nobody committed to. Capturing warms nothing, and that is
correct.

**Install `house_rules` in your own instructions file, once.** `CLAUDE.md`,
`AGENTS.md`, `.cursorrules` — whichever one you read at the start of every
session. It is the short version of this guide plus the workspace this project
writes to, and without it the next session starts blind: it opens a second thread
beside the one that was already half done, or writes nothing and the board says
nothing happened here.

`repos` says where the workspace's CODE lives — the repositories linked to it,
which are not the markdown tree this server writes. `inventory` says where the
things that keep it running live: servers, domains, services, with their provider
and renewal date, and only if you were given access. Both are READ ONLY, and both
have the same reason: linking a repository or buying a server is a decision
somebody makes in the app. If one is missing, `capture` it and say so.

**Neither hands you a credential.** The inventory's `vault` field is a link to
where a secret is kept — this server never holds the secret itself, so asking
again in another shape will not produce one.

`plan` shows the department's objectives and its inbox. The objectives belong to
the whole department rather than to a workspace — threads from any project hang
from the same one, which is what makes stating it worth anything — and shaping
them is the global lead's.

## What this server does not do

**Nothing here validates the shape of your document.** There is no template, no
required section, no linter. A thread needs a name; the document is yours.

How your team writes things down is your team's to decide, and it is written down
where they decided it: **read `README.md` in the workspace before you write
anything.** That is where conventions live, if there are any. If it is empty, there
are none.
