# Bubble Work WoW

This is the first dump on a new personal task tracking framework.

## Workspace

This is the first model to think of in this framework, a workspace can be an abstract grouping like a project, termporal grouping context
(a day, week, month, year, etc.), a workspace is where bubbles are going to live.

## Bubbles

Bubbles are also an abstract group model, but this time what bubles are intended to group are threads of work, the importance of
thinking it like a bubble is that a bubble may go up or down as air or any force is interacting with it, that up down thinking should
be this:

- If bubble goes up: means that bubble is fresh and is a work in progress
- If bubble goes down: time is taking on the bubble and the work is being defered

As you may assume from our thinking of up / down bubbles, is that time is the one governing this up / down behavior, and what the
person using this bubble framework should take care of the bubbles being shown at the top, and if the time comes, revive a stale
bubble (being down) and refresh it to move it up again. The way of refreshing a bubble should also be automatic but also support a
user manually pushing the bubble up, the automatic behavior is basically just publishing work into a bubble, that slowly makes
bubble to push up as the person publishes work.

## Threads

This is what lives inside a bubble and the core units of work, a thread's birth should always be done by 2 main artifacts:

1. BRIEF.md: this artifact explains what the task is all about, explaining purpose, maybe current state of the project, some project
    specific concepts to touch / refine, and other stuff that may be relevant to explain why this task should be done. And a MUST
    have section is Definition of Done.

2. LOGBOOK.md: as its name says, this is where the person should publish its advancements and progress, but this logbook should be done
    as a pair document of BRIEF.md, here the task must be decomposed into various phases, each phase should stand out what must be done
    at each. If task is small enough this artifact can be omitted and BRIEF.md can be closed by just a small paragraph explainded how
    it was achieved / done.
