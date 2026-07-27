Yes. The smallest reliable setup is:

1.⁠ ⁠Put the framework’s operating rules in a repository-level AGENTS.md.


2.⁠ ⁠Create a CLAUDE.md containing only @AGENTS.md.


3.⁠ ⁠Connect both agents to Plane through Plane’s MCP server.



Codex automatically reads AGENTS.md before working. Claude Code normally reads CLAUDE.md, but Anthropic explicitly supports importing AGENTS.md from it, allowing one shared instruction source. 

Recommended Plane mapping

I would slightly change your proposed mapping:

Your concept	Plane representation	Reason

Workspace	Project	Plane already uses “Workspace” for the organization-level container
Bubble	Module	Modules are durable logical groupings of related work
Bubble heat	Cycle activity	Cycles provide the time window in which activity occurs
Thread	Work item	The executable unit of work
Brief	Work-item description or linked Page	Holds intent, context, constraints and acceptance criteria
Tracking artifact	Child work items/sub-work-items	Keeps execution and status inside Plane


Plane defines a Workspace as its top-level container, with Projects underneath it. Projects contain work items, modules, cycles and pages. Modules are logical groupings, while Cycles are time-boxed periods. Because of that, a Bubble that can remain alive indefinitely fits a Module better than a Cycle. 

A clean interpretation would be:

	⁠The Bubble is the persistent body of work; Cycles are the pulses that keep it warm.



If you represent the Bubble directly as a Cycle, you should treat each cycle as a new “generation” of the Bubble rather than keeping the same cycle alive forever.

Minimal agent configuration

Create:

your-repository/
├── AGENTS.md
├── CLAUDE.md
└── .codex/
    └── config.toml

CLAUDE.md only needs:

@AGENTS.md

Here is a compact starting point for AGENTS.md:

Work Management Protocol

Plane is the source of truth for work status, planning and execution.

Vocabulary

•⁠  ⁠Framework Workspace: the mental boundary for a body of work. It normally maps to a Plane Project. Do not confuse it with a Plane Workspace.
•⁠  ⁠Bubble: a durable grouping of related work. It maps to a Plane Module.
•⁠  ⁠Heat window: the current Plane Cycle used to measure recent meaningful activity in a Bubble.
•⁠  ⁠Thread: an executable work element inside a Bubble. It maps to a Plane Work Item.

Thread birth rule

A Thread must not enter implementation until both birth artifacts exist:

1.⁠ ⁠Brief
   
   - Problem or opportunity
   - Intended outcome
   - Constraints and relevant context
   - Definition of done
   - Links to relevant source material

2.⁠ ⁠Tracker
   
   - At least one actionable todo
   - Current owner
   - Current state
   - Dependencies or blockers when applicable

The Brief should live in the Plane work-item description or in a linked Plane Page.

The Tracker should use Plane sub-work-items whenever possible. Do not create a second tracking system in repository files unless Plane cannot represent the required information.

Working protocol

Before starting work on a Thread:

1.⁠ ⁠Read the corresponding Plane work item.
2.⁠ ⁠Identify its Bubble, Brief and Tracker.
3.⁠ ⁠Create or repair missing birth artifacts before implementation.
4.⁠ ⁠Confirm the intended outcome and definition of done.

During work:

•⁠  ⁠Update the Tracker when the plan materially changes.
•⁠  ⁠Record blockers and important decisions.
•⁠  ⁠Link concrete evidence such as commits, pull requests, documents, builds or released artifacts.

At the end of work:

•⁠  ⁠Update completed and remaining todos.
•⁠  ⁠Add links to produced artifacts.
•⁠  ⁠Record the next concrete action.
•⁠  ⁠Change the Thread state only when the Plane state accurately reflects reality.

Bubble temperature

Only meaningful outputs generate heat. Meaningful outputs include:

•⁠  ⁠A completed todo
•⁠  ⁠A committed or reviewed implementation
•⁠  ⁠A published deliverable
•⁠  ⁠A recorded decision
•⁠  ⁠Validated user or stakeholder feedback
•⁠  ⁠Removal of a material blocker

Comments, status pings and cosmetic edits do not generate heat by themselves.

Classify Bubbles relative to Cycles:

•⁠  ⁠Hot: meaningful output occurred during the current Cycle.
•⁠  ⁠Warm: meaningful output occurred during the previous Cycle and active Threads remain.
•⁠  ⁠Cooling: no meaningful output occurred during the current or previous Cycle.
•⁠  ⁠Dormant: no meaningful output occurred for two or more Cycles, or the Bubble has no active owner.
•⁠  ⁠Closed: the intended outcome was reached or explicitly abandoned.

Do not create activity solely to keep a Bubble warm. Recommend closing, pausing or redefining a Bubble when its intended outcome no longer justifies continued work.This is enough for both tools to understand your model without injecting a long methodology into every prompt.

Connecting Plane

Plane now provides an MCP server that exposes its API as agent tools, including creating and updating work items and managing cycles. 

Claude Code

claude mcp add --transport http plane https://mcp.plane.so/http/mcp

Claude Code should open the Plane OAuth flow. You can inspect the connection using:

claude mcp list

Plane documents this exact Claude Code configuration. 

Codex

Put this in ~/.codex/config.toml, or .codex/config.toml for a trusted project:

[mcp_servers.plane]
url = "https://mcp.plane.so/http/mcp"
auth = "oauth"
default_tools_approval_mode = "writes"

Then authenticate:

codex mcp login plane

Codex supports streamable HTTP MCP servers with OAuth, and its CLI, desktop app and IDE extension share the same MCP configuration. The writes approval mode means write operations can require approval while reads remain less intrusive. 

What I think of the framework

The framework is strong as an attention model. “Bubbles warming and cooling” is more intuitive than another backlog hierarchy, and requiring a Brief plus Tracker before a Thread is born is an excellent constraint. It prevents agents from turning vague ideas directly into implementation.

The main risks are:

Terminology collisions. Plane already has Workspaces and software engineering already has threads. Keep the metaphor in the human-facing layer, but give every concept an explicit Plane mapping.

Activity gaming. If every comment warms a Bubble, the framework rewards motion rather than progress. Heat should require evidence of changed reality.

Zombie Bubbles. A Bubble needs a cooling and death mechanism, not only a way to remain alive.

Duplicated artifacts. Briefs and todos should usually live inside or alongside the Plane work item, rather than being duplicated in Markdown, Plane and code comments.

Missing outcome semantics. Each Bubble should have an intended outcome, owner and explicit closure condition. Otherwise it becomes only a thematic folder.


My strongest recommendation is to treat your framework as:

	⁠Workspace = boundary, Bubble = attention, Cycle = pulse, Thread = execution, Artifacts = evidence.



Start with the single AGENTS.md, the import in CLAUDE.md, and Plane MCP. Introduce a reusable Claude/Codex skill such as birth-thread only after you have repeated the process enough to know which steps are genuinely stable. Codex skills are loaded on demand rather than placing their full instructions into every interaction, making them suitable for that later stage.
