---
name: create-a-thread
title: Create a thread
description: Create a thread and write its document in whatever shape the work actually has.
args: bubble_id (required), subject
---

Create a thread in bubble `{{bubble_id}}`{{#subject}} for: {{subject}}{{/subject}}.

## What is required

A **name**. That is the whole gate.

`create_thread` takes `bubble_id`, `name`, and an optional `body` — your document, in
Markdown, written exactly as you send it. No headings are added around it, nothing is
graded, nothing is refused for its shape.

This is deliberate (`docs/decisions/0005`). The tool's job is to hold the writing and
tell you what has gone cold, not to grade the paperwork. Write the thread the way the
work actually has shape.

## What is worth writing anyway

Nobody is forced into this, and it is still the difference between a thread that
finishes and one that drifts:

- **why this exists**, in a sentence somebody else could act on
- **what is true when it is done** — a finish line, however you phrase it
- **the next concrete action**, so the version of you that returns in three weeks does
  not have to re-derive it
- **checkboxes** for the steps: `- [ ] …`. Ticking one is the smallest piece of
  evidence this thread can produce, and evidence is what keeps it warm.

Checkboxes count **anywhere in the document** — under your own headings, in your own
order. You do not need a section called Logbook to have a plan.

## If you want the two headings that do something

Two sections have machinery attached, and using them buys you something concrete:

| Heading | What it buys |
|---|---|
| `## Definition of Done` | `complete_thread` reports which items are still open when you finish; `audit_bubble` shows progress against it |
| `## Logbook` | its own addressable region, so a plan can be rewritten without touching the rest of the page |

Use them if they help. Skip them if they do not. If you do use them, write each one
**once** — a second `## Logbook` on the same page truncates the first, and that IS
refused, because every region-scoped write then addresses nothing.

Everything else is prose the tool does not read. There is no Brief template, no
Context / Outcome / Symptom / Repro / Scope — those were removed
(`docs/decisions/0006`) because a template that fits every kind of work fits none of
them well.

## To categorise the work, use Plane

Type, area, severity, "needs design" — that is what **labels** are for, and Plane
already has them. Do not encode it in a heading or a line of prose; nothing parses it
there, and a label is filterable everywhere in Plane as well as here.

## Then

`create_thread` returns the thread id, and says what it noticed — a missing finish
line, a document with no checkboxes — having created the thread regardless.

Creating a thread is production: it is 🔥 and its bubble rises. You have already
changed something by defining the work.
