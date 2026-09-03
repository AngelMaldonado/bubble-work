# MCP — agents as members

**Status:** built (steps 1–3, 5) · **Packages:** `internal/mcpapi`,
`internal/server/mcphost.go` · **Depends on:**
[`identity.md`](./identity.md) · **History:**
[`journal/MCP-ACCESS.md`](../journal/MCP-ACCESS.md)

## What it solves

Agents (Claude Code, Codex) are members of the organization, not scripts bolted on
the side. They read threads, write artifacts, tick todos and finish work through the
same server methods a human uses, so the framework's rules apply to them
identically.

## Model (normative)

The server exposes its **own** MCP endpoint at `/mcp` — **not** Plane's. An agent
authenticates by presenting a human's Plane API key as its Bearer credential, and
thereby *is* that human ([`identity.md`](./identity.md)).

```bash
claude mcp add --transport http bubble http://localhost:4006/mcp \
    --header "Authorization: Bearer <your plane API key>"
```

The web UI's **Connect** panel generates that command, plus a setup prompt, with the
key masked until asked for.

MCP calls server methods **directly** rather than looping back over HTTP, which is
why the surfaces cannot drift: same method, same policy engine, different door.

### The tools

30 tools, grouped by what they do:

| | Tools |
|---|---|
| **Read** | `list_workspaces` `list_bubbles` `audit_bubble` `thread_timeline` `read_thread` `thread_comments` `list_pages` `read_page` |
| **Create** | `create_workspace` `create_bubble` `create_thread` (alias `birth_thread`) `add_revision` `create_page` `add_link` |
| **Change** | `update_thread` `toggle_todo` `delete_artifact` `set_contract` `move_thread` `rename_bubble` `rename_workspace` `update_page` |
| **Finish** | `complete_thread` `reopen_thread` |
| **Discuss** | `post_comment` `mark_comments_read` |
| **End** | `close_bubble` `delete_bubble` `delete_thread` `delete_page` `delete_workspace` |

### Prompts

Four MCP prompts ship with the server, as **markdown files** under
[`prompts/`](../../prompts) embedded with `go:embed` — the same trick `web/embed.go`
plays with the SPA bundle. Files rather than string constants because prose that has
to be re-escaped into Go source stops getting edited, and these are meant to be tuned.

| Prompt | What it carries |
|---|---|
| `bubble-work` | the model: vocabulary, what warms and what does not, the region contract |
| `create-a-thread` | what a thread actually needs, what is worth writing anyway, and the templates on offer |
| `work-a-thread` | read → repair → edit → record, and which field is quotable |
| `finish-a-thread` | the DoD gate, and what `force` is actually for |

Each file has a small frontmatter block (`name`, `title`, `description`, `args`) and a
body with `{{arg}}` substitution plus `{{#arg}}…{{/arg}}` for optional passages. A
malformed file is logged and skipped rather than fatal — a typo in prose must not take
down the tool surface, which this package has already learned once.

There is no runtime reload, deliberately: what an agent was told has to be
reproducible from the commit that was deployed.

### Schemas are closed where the behaviour is

A field with a fixed set of legal values is declared as an **enum**, not as a string
the runtime rejects later: `create_thread.type`, and `region` on `toggle_todo` and
`delete_artifact`. The SDK's `jsonschema:"…"` tag only sets a description, so those
tools pass an explicit `InputSchema` built by `schemaFor` + `enumOn`.

This came from a real agent session that sent an invalid `type` and learned the
constraint by being refused. A constraint the client can enforce belongs in the
schema.

### Rules that bite agents specifically

- `set_labels` / `relate_threads` / `add_link` are where "what kind of work is this",
  "what does it depend on" and "where is the evidence" live — not in the prose
  ([`decisions/0006`](../decisions/0006-plane-holds-the-relationships.md)). Only
  `add_link` is production.
- `create_thread` needs a name, and nothing else. `body` is the document, verbatim;
  `type` (bug | feature | chore) scaffolds a Brief for callers who want one — its
  schema is `domain.BirthRequest` itself, so the field arrived for free.
- `update_thread` prefers surgical `edits` (quote the old text, give the new) over
  wholesale section replacement — the failure mode being an agent overwriting a
  paragraph it never read.
- `toggle_todo` refuses the write if the item text no longer matches that position.
- `complete_thread` is never refused; it reports open Definition of Done items in
  `unmet_dod` ([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)).
- Destructive tools require the exact current name as confirmation.
- `post_comment` earns no heat, and agents are told so: a comment is presence.
- **Write each reserved heading at most once** (`## Brief`, `## Logbook`,
  `## Definition of Done`). A second one is refused, because the first section truncates
  at the second and its region becomes unreachable
  ([`artifacts.md`](./artifacts.md)).
- `read_thread` returns several views of one page and only `regions[…].markdown` is
  what a write diffs against. The tool descriptions say so, in those words.
- `toggle_todo` takes `items` for a batch — one call, one Plane write, all-or-nothing
  — and answers with a **compact confirmation** (each item as persisted, the per-region
  counts, what still blocks completion, the thread's level) instead of the whole
  interior. Returning the full thread technically answered "did it land?" and
  practically did not: the answer was buried, so callers re-read the thread.
- `audit_bubble` answers the framework's questions for a whole bubble in one call, so
  an agent does not walk `thread_timeline` + `read_thread` per thread. It costs no
  Plane calls, and it reports what is MISSING per thread — the finding nobody gets by
  reading threads one at a time.

### Liveness

An agent's write pushes an SSE nudge, so a human watching that thread in the browser
repaints while the agent is editing instead of saving over it. That is the whole of
"real time" here — two lags, the agent-to-server one (now zero, since writes are
local) and the server-to-browser one (SSE).

### Robustness

One malformed tool definition once took the whole MCP surface down; tools are now
registered defensively so a single bad schema cannot deny the others.

## Invariants

1. Agents consume **our** MCP, never Plane's.
2. An agent has exactly the scope of the human whose key it holds.
3. Every rule is enforced in the server method, so no tool can bypass one.
4. A tool that fails must not take the surface with it.

## Open

- The prompts are only reachable over MCP. A `bubble prompts` command (or an admin
  endpoint) would let a human read what agents are being told without an MCP client.
- A stable public HTTPS URL and an OAuth authorization server, for hosted clients
  that cannot paste a header (`journal/MCP-ACCESS.md` steps 4 and 6).
- No agent-level audit trail, by construction ([`identity.md`](./identity.md)).
