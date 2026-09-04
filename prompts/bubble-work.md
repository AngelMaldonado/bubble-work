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
  README.md          the workspace's front page
  threads/           one file per thread, FLAT — no subdirectories
  docs/              the wiki: guides, references, notes. Nests freely.
```

Only `.md` and `.excalidraw` are writable. Anything outside this shape is refused.

A thread's file is named by the server and moves when the thread is renamed. Do not
depend on a path staying still; a thread id does.

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

## What this server does not do

**Nothing here validates the shape of your document.** There is no template, no
required section, no linter. A thread needs a name; the document is yours.

How your team writes things down is your team's to decide, and it is written down
where they decided it: **read `README.md` in the workspace before you write
anything.** That is where conventions live, if there are any. If it is empty, there
are none.
