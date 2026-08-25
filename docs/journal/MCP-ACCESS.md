# MCP access — agents acting, artifacts updating live

Status: **Steps 1-3 and 5 built · 4 and 6 pending** · Drafted 2026-08-06 · Companion to
[`AGENTS.md`](../../AGENTS.md) and [`PLANE-SYNC.md`](./PLANE-SYNC.md).

> **Journal.** This is the record of how the work went — the problem as it was
> measured, the phases, what the live instance changed. It is kept as written and
> is not updated when the system moves on. For what is true today, read [`modules/mcp.md`](../modules/mcp.md).

## The intention

An agent works through the MCP endpoint — updating a Logbook, ticking a todo,
recording a decision — and the person watching the board sees the artifact change
as it happens. Signing an agent in should be a link, not a pasted key.

## What is actually in the way

Four things, found by reading the code and probing rather than assuming.

### 1. No MCP tool can edit an artifact

This is the big one, and it blocks the intention outright. The ten tools are:

```
list_bubbles   create_bubble   birth_thread    set_contract   close_bubble
thread_timeline  read_thread   thread_comments  post_comment  mark_comments_read
```

`birth_thread` *creates* a Brief and Logbook. Nothing **updates** one. `set_contract`
writes the server's own overlay (outcome/owner/closure), not the Plane artifact.
So an agent can create a thread and talk about it, and can change nothing.

Worth noting what this means for heat: `THREAD-LIFECYCLE.md` treats a Logbook
edit as the primary evidence of production. The MCP surface currently cannot
produce evidence at all, which makes an agent structurally incapable of warming
a thread.

**Needed:** an `update_thread` tool that writes `description_html`, and probably
`add_revision` (a sub-work-item) since revisions are the other evidence kind.

### 2. "Real time" has two separate lags

Even with a write tool, a change would take up to **two minutes** to appear, and
then only if you were looking at the board:

- **Mirror lag.** Since `PLANE-SYNC.md` Phase 2 the UI reads the local mirror,
  which polls Plane on `DeltaInterval` (2 min). A write through MCP goes to
  Plane and the UI does not see it until the next delta.
- **The open thread never re-fetches.** The SSE stream nudges the *board*
  (`store.refresh()`), but `ThreadView` loads its detail once on open. Watching a
  Logbook change live is exactly the case that does not update.

**Needed:** the MCP write path updates the mirror directly (the same trick
`PostComment` already uses for a posted comment — Plane accepted it and returned
the row, so writing it locally is exact, not optimistic), and the SSE nudge has
to reach an open thread, not just the board.

### 3. The server is not reachable from the internet

`bubble.angel.cubytest.space` resolves to **192.168.20.102** — a private address.
It answers from the mini and from the LAN, and not from anywhere else. So a
hosted client (claude.ai) cannot reach it at all, and even Claude Code can only
reach it from inside the network.

A tunnel is genuinely needed. Two candidates:

| | ngrok | Tailscale Funnel |
|---|---|---|
| account | new one | **already running** (the mini is on the tailnet) |
| URL | random per session unless you pay for a static domain | stable `*.ts.net`, free |
| cert | theirs | theirs, valid |
| cost | free tier churns URLs | free |
| shape | separate agent process | `tailscale funnel 3104` |

**Recommendation: Tailscale Funnel.** A churning URL is not a small problem here
— an MCP server's identity *is* its URL: it goes in every client's config, and
OAuth binds tokens to it as the audience (RFC 8707). A URL that changes every
restart invalidates both. ngrok solves that only on a paid plan; Funnel is stable
and free and needs no new account.

Verified on the mini: **Tailscale 1.98.8**, and `funnel status` answers
("No serve config" — available, not yet configured). The CLI is not on `PATH`;
it lives at `/Applications/Tailscale.app/Contents/MacOS/Tailscale`.

Two things still to confirm before committing to it, neither of which I changed:

- Funnel must be **enabled for the tailnet** — it needs HTTPS certificates on and
  the `funnel` node attribute in the ACL policy. That is an admin-console change
  and it is yours to make.
- Funnel exposes ports 443/8443/10000 only, so it would front the service rather
  than replacing the existing `bubble.angel.cubytest.space` LAN name. Both can
  coexist: LAN clients keep the short name, external ones use the `ts.net` URL.

If the ACL turns out to be awkward, **Cloudflare Tunnel** is the other stable-URL
option and does not depend on the tailnet at all.

### 4. OAuth is a bigger lift than it looks

The MCP spec (2025-11-25) says the server is an OAuth 2.1 **resource server**, not
an authorization server. Concretely it must:

- serve `/.well-known/oauth-protected-resource` (RFC 9728) naming an
  `authorization_servers` entry;
- answer an unauthenticated request with
  `401 WWW-Authenticate: Bearer resource_metadata="…"`;
- **validate that the token's audience is this server** (RFC 8707) — accepting a
  token minted for someone else is the vulnerability the rule exists to prevent.

The Go SDK already ships the resource-server half: `auth.RequireBearerToken` is
in use today, and `auth.ProtectedResourceMetadataHandler` is sitting unused.

The missing half is an **authorization server** — `/authorize`, `/token`, PKCE,
and either Dynamic Client Registration or Client ID Metadata Documents. That is
the real work, and nothing in the stack provides it: Plane does not do OAuth for
third parties, so there is no existing IdP to delegate to without adding one
(Google/GitHub).

## Three ways to do the sign-in, honestly costed

**A — Connect page (no OAuth).** A page in the web UI, where you are already
signed in, that mints a scoped server token and shows the exact
`claude mcp add --transport http … --header` command to paste. One click, one
paste. No spec compliance, no AS, works with Claude Code today.

**B — Minimal OAuth 2.1 AS in the binary.** `/authorize` reuses the existing web
session (you are signed in with your Plane key), you press Approve, and the
server issues an audience-bound token. Plus PKCE, DCR, and the metadata
documents. Spec-compliant, one-click from any MCP client including hosted ones.
Roughly 400–600 lines and a security surface worth being careful with — token
signing, redirect-URI validation, code replay.

**C — Delegate to Google/GitHub.** Standard AS, no crypto of ours. But it adds a
second identity system next to the Plane key, and someone with a Google account
is not necessarily a Plane member, so it needs a mapping step and a way to refuse
strangers.

**Recommendation: A now, B when a hosted client needs it.** A is an afternoon and
unblocks testing today; the artifact-write tools and the live-update path are
where the actual value is, and they are independent of how you sign in. B is
worth doing when you want claude.ai (not just Claude Code) connecting, because
that is the case where a pasted header is not an option.

## Proposed order

1. [x] **`update_thread` + `add_revision` MCP tools.**
2. [x] **Write-through to the mirror on every MCP write.**
3. [x] **SSE reaches the open thread.**
4. **Tailscale Funnel** for a stable public HTTPS URL.
5. [x] **Connect page** (option A) for one-click Claude Code setup.
6. **OAuth AS** (option B) if and when a hosted client needs it.

Steps 1–3 are the intention. 4–5 are access. 6 is polish.

## Steps 1-3 as built

**`update_thread` is section-scoped, not a whole-body write.** That was the open
question and it answers itself once stated plainly: the Brief and the Logbook
share one Plane description by design (§3), so the obvious shape — hand the agent
the whole body — lets a plan revision erase the human's statement of intent. The
tool takes optional `brief` and `logbook`; a field you don't pass is untouched.
`md.ReplaceSection` swaps a section's content in place, keeping its heading
verbatim and its position in the document, and appends the section when a thread
was born without one (§3.2 allows a small thread to have no Logbook).

`TestReplaceSection` is really a blast-radius test, and
`TestUpdateThreadIsEvidenceAndImmediate` asserts the same thing end to end: after
an agent rewrites a Logbook through the API, the Brief is still there.

**Both writes go into the mirror immediately.** Not optimistic — Plane accepted
the write and returned the row, so this is what Plane holds. Without it the
server would wait up to a delta interval to rediscover a change it made itself.

**The SSE nudge now carries a thread id.** Empty means "the board changed", a
value means "these artifacts changed", so a client re-fetches a thread only when
it is the one on screen — rather than every client re-fetching on every board
tick for a thread that usually did not change.

**A Logbook edit is production, so it warms the thread.** That is the part that
makes the MCP surface worth having: before this an agent could create a thread
and comment on it and change nothing, which made it structurally incapable of
warming work it was actually doing.

One behaviour worth knowing: a thread whose *first ever* board build coincides
with an edit is baselined silently and does not warm. That is the documented
first-sighting rule (we cannot know when a logbook was last touched, and stamping
it "now" would fabricate heat for work that might be a year old). In production
the refresher establishes the baseline long before anyone edits; only a test can
arrive with both at once, which is exactly what the test had to be taught.

## Step 5 as built

An **MCP badge** in the top bar, left of the language toggle, opens a per-person
connect panel: the server URL (derived from wherever you are browsing, so it is
right for localhost and for the deployed host without configuration), your
token, and three ways to use them — the `claude mcp add` one-liner, a global
config block, and **a prompt to paste into an assistant** so it does the
configuration itself rather than you hunting for the right config path. The
prompt names no client: it gives the transport, the URL and the header, and
tells the assistant to work out the right config file for whatever client is
actually in use.

Nothing is minted. The token is the Plane key this browser already holds, which
is why option A needed no server work at all — but it also means the panel has
to say what it is handing over, so it does: *this is your personal Plane API
key, not a scoped token; anyone holding it can read and write as you*. That
sentence is the argument for step 6 whenever it comes.

Everything is masked until you press Show, including inside the snippets and the
prompt, because a credential on a screen is a credential in a screenshot.

## Still open

- Do agents write as themselves or as the human they impersonate? Today an agent
  presents the human's Plane key and is indistinguishable by design (§9.3) — an
  OAuth token would make it separable, which may or may not be wanted.
- `add_revision` picks the project's default state. Fine for a findings write-up;
  possibly wrong if revisions should be born in a specific column.
