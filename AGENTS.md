# Bubble Work — Agent Operating Rules

You are working inside the Bubble Work repository. The canonical model is
[`bubble-work-spec.md`](./bubble-work-spec.md) — read it before non-trivial work.
This file is the operational summary both Claude Code and Codex load.

## Vocabulary (keep the metaphor human-facing)

- **Workspace** — the boundary for a body of work. Maps to a Plane **Project**.
  Not a Plane "Workspace".
- **Bubble** — a durable grouping of related work; the unit of *attention*.
  Maps to a Plane **Module**. It has an outcome and can die.
- **Cycle** — the repeating pulse (heat window) recency is measured against.
- **Thread** — one executable unit of work. Maps to a Plane **work item**.
- **Heat** — evidence of *changed reality*, not activity.

## Thread birth rule (non-negotiable)

A thread must not enter implementation until BOTH birth artifacts exist:

1. **Brief** — problem/opportunity, intended outcome, constraints, links, and a
   required **Definition of Done** section.
2. **Logbook** — the plan decomposed into phases, plus ≥1 actionable todo, a
   current owner, and current state. Lives in the SAME Plane work-item
   description/page as the Brief (one page, not a second tracker).

Small threads may omit the Logbook and close the Brief with a short paragraph.

## Heat = evidence, never motion

Only meaningful outputs warm a bubble: a completed todo, a committed/reviewed
implementation, a published deliverable, a recorded decision, validated
feedback, or the removal of a material blocker. Comments, status pings and
cosmetic edits do NOT generate heat. Never manufacture activity to keep a bubble
warm — revive it with real work, redefine it, or close it.

## Lifecycle

Hot → Warm → Cooling → Dormant → Closed, computed against the Cycle. Recommend
closing or redefining a bubble when its outcome no longer justifies the work.

## Architecture (how this repo is built)

- One Go binary, two modes: `bubble serve` (the brain) and the thin client.
- **Plane is the system of record** for work items + activity. The **server**
  owns the overlay: heat, lifecycle, the Bubble contract, and members.
- The server is Plane's ONLY client, over REST only. No component talks to
  Plane directly or via Plane's MCP.
- Agents consume the server's OWN MCP endpoint (`/mcp`), authenticated as a
  member. The framework's rules are enforced server-side, so they cannot be
  bypassed from any client.

## Surface parity (default)

A server capability lands on **all three client surfaces in the same change**:
the REST API, the CLI thin client, and the MCP tools. The logic lives in a
server method that each surface reuses (CLI over HTTP, MCP calling the method
directly), so they never drift. The web UI follows when the capability has a
visual form. Only skip a surface when the capability is inherently specific to
one (e.g. mds rendering is UI-only) — and say so.

## Working protocol

Before: read the thread, identify its Bubble/Brief/Logbook, repair missing birth
artifacts, confirm the outcome + Definition of Done.
During: update the Logbook when the plan materially changes; record blockers and
decisions; link concrete evidence (commits, PRs, deliverables).
After: update todos, link artifacts, record the next concrete action, and change
state only when it reflects reality.
