# Identity — everyone is a Plane member

**Status:** built (Phase 4) · **Packages:** `internal/server` (actor resolution),
`internal/mirror` (members) · **Depends on:**
[`plane-channel.md`](./plane-channel.md) · **History:**
[`journal/PLANE-SYNC.md`](../journal/PLANE-SYNC.md) Phase 4

## What it solves

Attribution without a second account system. The framework's rule against
duplicated state applies to people too: if Plane knows who someone is, we must not
keep a copy of that answer.

## Model (normative)

Everyone — human or agent — authenticates with a **Plane API key**.

- **Humans use their own key.** The server verifies it via Plane's `/users/me`,
  then uses its **own** per-instance admin keys to find every instance whose member
  list contains that email. Instance scope and role (`admin` when Plane role ≥ 20)
  are therefore *derived*, not granted. No account is created for a human, ever.
- **Agents impersonate a human** by presenting that human's key. The agent *is*
  that person: same identity, same scope, same role, same Plane attribution on
  writes. There is no agent registry.

A request resolves to an **Actor** — id, name, kind, admin flag, and the instances
it may see — and every action is attributed to it.

### Why identity is not mirrored, but membership is

The two halves are split deliberately:

- **Identity** (this key belongs to this person) is verified against Plane and
  cached briefly. It is a credential check; a stale answer would be a security
  answer.
- **Membership** (which workspaces list this email) is mirrored, because it is
  structure and reading it live would cost a Plane call per request.

An **empty member table is not an answer.** If membership has not been mirrored
yet, "you belong to nothing" is a lie the server must not tell — it is the
difference between a permission decision and a sync state.

### Admins

Admin is Plane's role, plus a break-glass `$BUBBLE_ADMIN_TOKEN` for the server host
when Plane itself is what is broken. Kiosk tokens are a third, read-only kind of
caller for wall displays.

## Surfaces

- REST — `GET /api/whoami`; every endpoint resolves an Actor.
- CLI — `bubble whoami`, `bubble init`, `bubble use` (credential profiles are
  client-side; see [`operations.md`](../operations.md)).
- MCP — the `Authorization: Bearer <plane key>` header is the whole login.
- Web — the same key, held by the browser session.
- Admin — `GET /api/admin/members`, `bubble admin kiosk ls|new|rm`.

## Storage

`mirror_members`, `mirror_project_members`, `mirror_project_member_sync` (mirror);
`kiosk_tokens`, `member_prefs` (overlay). Credentials for *instances* live in
`plane_instances` and never leave the server.

## Invariants

1. No user records. Plane is the member list.
2. A human's key is never stored server-side; an instance's key never leaves it.
3. An unmirrored member list is a 5xx-class condition, not "unauthorized".
4. Every mutation is attributed to a resolved Actor.

## Open

- Agent-level audit is impossible by construction: an agent is indistinguishable
  from the human it impersonates, and revoking it means rotating that human's key.
  Accepted trade-off, revisitable only with real agent credentials.
- A cold server currently answers the third invariant wrongly, returning 403 where
  it means 503.
