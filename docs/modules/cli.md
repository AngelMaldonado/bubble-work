# CLI — the human surface

**Status:** built · **Packages:** `cmd/bubble`, `internal/client` ·
**Depends on:** the REST API (the CLI is an HTTP client, never a Plane client)

## What it solves

Doing the work from a terminal, at the same fidelity as the board — because most
work happens next to a shell, not next to a browser tab.

## Model (normative)

The CLI is a **thin client over REST**. It holds no policy: every rule lives in the
server method behind the endpoint, so the CLI cannot enforce a rule the MCP surface
lacks, or vice versa.

Ids are namespaced (`slug:project:object`) and **any unique prefix is accepted**,
which is what makes them typable.

Configuration lives in `$BUBBLE_HOME` (default `~/.bubble/`): one `config.json`
holding one or more **credential profiles**, each carrying both a Plane key and a
server URL, so `bubble use` switches identity and deployment in one move.

## The commands

```
# server
bubble serve [--addr :4006]      run the server (REST + MCP + web brain)
bubble stop                      stop the running server (graceful)
bubble attach                    follow the running server's log (Ctrl-C detaches)

# looking around
bubble ls                        list bubbles, hottest first (buoyancy view)
bubble heat <id>                 explain a bubble's temperature
bubble show <id>                 a bubble's thread timeline (git-log-oneline)
bubble audit <id>                a bubble's whole state: DoD progress, next actions,
                                 and what each thread is missing
bubble thread <id> [--comments]  a thread's interior: artifacts, logbook, revisions
bubble search <query>            fuzzy-search threads across your instances
bubble whoami                    the identity resolved from your credential

# doing the work
bubble new <id> [flags]          create a thread in a bubble (alias: birth; needs
                                 only --name; --body is your document, any shape)
                                 --type bug|feature|chore shapes the Brief
                                 --template prints that shape and exits
bubble logbook <id> [text|-]     rewrite a thread's Logbook (evidence → warms it)
bubble dod <id> [text|-]         rewrite a thread's Definition of Done
bubble section --id <id> --title <t> [--file <f>|-] [--delete]
                                 add / rewrite / remove a ## section of a thread
bubble todo <id> <n> done <text> tick the nth todo (text guards the position)
bubble revision <id> <title> [-] attach a revision artifact to a thread
bubble comment <id> <text...>    post a comment (as you; earns no heat)
bubble rename <id> <title>       retitle a thread or a revision
bubble move <thread> <bubble>    re-home a thread into another bubble
bubble done <id>                 mark a thread finished (🏆; reports any unmet DoD)
bubble reopen <id>               put a finished thread back to work

# structure
bubble workspace new|list|rename a Plane project — our Workspace
bubble bubble new|set|close|open create a bubble or set its contract
bubble bubble rename <id> --name retitle a bubble (the handle, not the contract)
bubble bubble review|unreview    the explicit 👀 stage overlay
bubble label <id> [name...]      set a thread's Plane labels (no args clears them)
bubble link <id> <url> [title]   attach external evidence (warms the thread)
bubble link rm <id> <link-id>    detach a link
bubble relate <id> <other> [type]  relate two threads; "rm" drops the relation
bubble page list|read|new|edit   a workspace's pages
bubble delete <kind> <id> [-y]   PERMANENTLY delete a workspace/bubble/thread/artifact

# attention
bubble notifications             your inbox of cooling/dormant alerts (alias: inbox)
bubble notifications on|off      opt in/out of notifications
bubble notifications read <id|all>  mark notifications read
bubble tick                      sweep now for cooling bubbles

# setup
bubble init [flags]              configure server URL + a credential profile
bubble use [name]                switch active credential profile (no arg: list)
bubble reset [--force]           purge all local state and start from scratch
bubble version

# server-host admin
bubble instance add|list|remove [--force]
bubble admin export <inst>       write the work to disk as markdown (--dir <abs path>)
bubble admin adopt <inst>        take its bodies into the document store (once)
bubble admin <cmd>               see operations.md
```

`bubble new` (alias `bubble birth`) asks for `--name` and nothing else. `--body` /
`--body-file` write the document as sent — `-` reads stdin — and `--brief`/`--logbook`
still produce the older sectioned shape. `--type` with `--template` prints a Brief
skeleton to fill in. `--small` is accepted and ignored: nothing is required, so
nothing needs excusing
([`decisions/0005`](../decisions/0005-the-framework-does-not-own-your-format.md)).

Reads print the derived level as a mark (🔥 😴 🪦 🏆 👀) rather than a status word,
and `bubble ls` prints a staleness warning above the list when the mirror is ageing
or the caller has unsent drafts.

Reads also say where a body came from: a thread last edited in Plane prints a
`body: last written in Plane (imported)` line ([`documents.md`](./documents.md)).

## Invariants

1. The CLI talks only to the server, never to Plane.
2. No rule is implemented here that is not implemented server-side.
3. Every capability the server gains lands here in the same change, unless it is
   inherently visual.

## Open

- No `bubble type <thread> <kind>` — a thread's type can only be set at creation.
