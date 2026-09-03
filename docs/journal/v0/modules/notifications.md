# Notifications — the scheduler that tells you what sank

**Status:** built · **Packages:** `internal/server` (`Tick`, the scheduler),
`internal/store` (`notifications`, `notification_reads`, `member_prefs`) ·
**Depends on:** [`heat.md`](./heat.md), [`bubbles.md`](./bubbles.md)

## What it solves

Buoyancy is only useful if someone notices the sinking. Heat is derived, so nothing
"happens" when a bubble cools — the number simply changes. The sweep is what turns a
silent transition into an event worth reading.

## Model (normative)

A **tick** classifies every bubble the server can see and records a notification when
a bubble crosses into Cooling or Dormant, or when a thread crosses into 🪦. It runs on
the scheduler and can be forced (`POST /api/tick`, `bubble tick`,
`bubble admin tick`).

Two properties make it liveable:

- **Transitions, not states.** A bubble that has been dormant for a month is not news
  every tick. The sweep records the crossing.
- **Per-member preferences and read receipts.** Notifications are opt-in per member
  (`member_prefs`), and reading one is recorded (`notification_reads`) rather than
  deleting it — so an inbox can be re-read.

Comment notifications work off `comment_reads` for the same reason: unread is a
per-member fact, not a property of the comment.

Because heat is a pure function, the sweep is **advisory**: skipping a tick loses a
notification, never a level. Nothing about the board depends on the scheduler having
run.

## Surfaces

| Capability | REST | CLI | MCP |
|---|---|---|---|
| inbox | `GET /api/notifications` | `bubble notifications` (alias `inbox`) | — |
| mark read | `POST /api/notifications/read` | `bubble notifications read <id\|all>` | — |
| opt in / out | `POST /api/notifications/prefs` | `bubble notifications on\|off` | — |
| force a sweep | `POST /api/tick` · `POST /api/admin/tick` | `bubble tick` · `bubble admin tick` | — |
| comment read receipts | `POST /api/threads/{id}/comments/read` | — | `mark_comments_read` |

No MCP tools for the inbox: an agent has no attention to manage. That is a
deliberate gap, not a missing surface.

## Storage

`notifications`, `notification_reads`, `member_prefs`, `comment_reads` — all overlay.

## Invariants

1. A notification marks a **transition**, never a standing state.
2. Read is per member, and never deletes the notification.
3. The sweep is advisory: a missed tick never changes a level.

## Open

- **A discussed bubble still gets its "cooling" notification.** The comment pulse
  floors the BAND at 😴 ([`decisions/0004`](../decisions/0004-writing-is-the-work.md))
  but leaves the lifecycle alone, and notifications key off the lifecycle. Defensible
  — *it went quiet; nobody said it was dead* — but the board and the inbox disagree in
  that one case.
- No delivery channel other than the inbox — no email, no push, no webhook out.
- The sweep walks every visible bubble each tick; fine at current sizes, worth
  measuring at scale.
