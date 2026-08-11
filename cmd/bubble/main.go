// Command bubble is the single Bubble Work binary. `bubble serve` runs the brain
// (server + MCP); the other subcommands are the thin client (§9).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/client"
	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/server"
	"github.com/AngelMaldonado/bubble-work/internal/store"
	planesync "github.com/AngelMaldonado/bubble-work/internal/sync"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "stop":
		cmdStop(os.Args[2:])
	case "attach":
		cmdAttach(os.Args[2:])
	case "ls":
		cmdLs(os.Args[2:])
	case "heat":
		cmdHeat(os.Args[2:])
	case "show":
		cmdShow(os.Args[2:])
	case "thread":
		cmdThread(os.Args[2:])
	case "comment":
		cmdComment(os.Args[2:])
	case "logbook":
		cmdLogbook(os.Args[2:])
	case "dod":
		cmdDoD(os.Args[2:])
	case "section":
		cmdSection(os.Args[2:])
	case "rename":
		cmdRename(os.Args[2:])
	case "move":
		cmdMove(os.Args[2:])
	case "done":
		cmdDone(os.Args[2:])
	case "reopen":
		cmdReopen(os.Args[2:])
	case "delete", "rm":
		cmdDelete(os.Args[2:])
	case "todo":
		cmdTodo(os.Args[2:])
	case "revision":
		cmdRevision(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	case "whoami":
		cmdWhoami(os.Args[2:])
	case "notifications", "inbox":
		cmdNotifications(os.Args[2:])
	case "tick":
		cmdTick(os.Args[2:])
	case "use":
		cmdUse(os.Args[2:])
	case "birth":
		cmdBirth(os.Args[2:])
	case "page", "pages":
		cmdPage(os.Args[2:])
	case "workspace":
		cmdWorkspace(os.Args[2:])
	case "bubble":
		cmdBubble(os.Args[2:])
	case "instance":
		cmdInstance(os.Args[2:])
	case "admin":
		cmdAdmin(os.Args[2:])
	case "init":
		cmdInit(os.Args[2:])
	case "reset":
		cmdReset(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("bubble-work %s\n", version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `bubble — Bubble Work

Usage:
  bubble serve [--addr :4006]      run the server (REST + MCP brain)
  bubble stop                      stop the running server (graceful)
  bubble attach                    follow the running server's log (Ctrl-C detaches)
  bubble ls                        list bubbles, hottest first (buoyancy view)
  bubble heat <id>                 explain a bubble's temperature (id from 'ls')
  bubble show <id>                 a bubble's thread timeline (git-log-oneline)
  bubble thread <id> [--comments]  a thread's interior: artifacts, logbook, revisions
  bubble logbook <id> [text|-]     rewrite a thread's Logbook (evidence → warms it)
  bubble dod <id> [text|-]         rewrite a thread's Definition of Done
  bubble rename <id> <title>       retitle a thread or a revision
  bubble move <thread> <bubble>    re-home a thread into another bubble
  bubble done <id> [--force]       mark a thread finished (🏆; refused if the DoD is unmet)
  bubble reopen <id>               put a finished thread back to work
  bubble delete <kind> <id> [-y]   PERMANENTLY delete a workspace/bubble/thread/artifact
  bubble todo <id> <n> done <text> tick the nth todo (text guards the position)
  bubble revision <id> <title> [-] attach a revision artifact to a thread
  bubble comment <id> <text...>    post a comment to a thread's discussion (as you)
                                   --retry/--discard <draft> for an unsent one
  bubble search <query>            fuzzy-search threads (tasks) across your instances
  bubble whoami                    show the identity resolved from your credential
  bubble notifications             your inbox of cooling/dormant alerts (alias: inbox)
  bubble notifications on|off      opt in/out of notifications
  bubble notifications read <id|all>  mark notifications read
  bubble tick                      sweep now for cooling bubbles
  bubble use [name]                switch active credential profile (no arg: list;
                                   --tokens shows keys masked, add --reveal for full)
  bubble section --id <id> --title <t> [--file <f>|-] [--delete]
                                   add / rewrite / remove a ## section of a thread
  bubble page list|read|new|edit   a workspace's docs and specs (Plane pages)
  bubble workspace new|list|rename  a Plane project — our Workspace
  bubble birth <id> [flags]        create a thread in a bubble (needs Brief + Logbook)
  bubble bubble new|set|close|open create a bubble or set its contract (§4)
  bubble instance add|list|remove  manage Plane instances (run on the server host)
  bubble admin <cmd>               service-admin ops (godmode; needs admin token/email)
  bubble init [flags]              configure server URL + a credential profile
  bubble reset [--force]           purge all local state and start from scratch

Everyone authenticates with a Plane API key — identity, role and instance scope
are discovered from Plane. Store one key + server per workspace/identity as a
named profile (`+"`bubble init --name cuby --token <key> --server <url>`"+`) so
`+"`bubble use cuby`"+` switches BOTH the credential and the server at once. An
agent impersonates a human by using that human's key.

`)
}

func cmdSearch(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: bubble search <query>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Search(cfg, strings.Join(args, " ")); err != nil {
		log.Fatalf("search: %v", err)
	}
}

func cmdWhoami(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.Current != "" {
		fmt.Printf("profile   : %s\n", cfg.Current)
	}
	fmt.Printf("server    : %s\n", cfg.ActiveServer())
	if err := client.Whoami(cfg); err != nil {
		log.Fatalf("whoami: %v", err)
	}
}

func cmdNotifications(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if len(args) == 0 {
		if err := client.Notifications(cfg); err != nil {
			log.Fatalf("notifications: %v", err)
		}
		return
	}
	switch args[0] {
	case "on":
		err = client.SetNotifyPref(cfg, true)
	case "off":
		err = client.SetNotifyPref(cfg, false)
	case "read":
		if len(args) < 2 {
			log.Fatal("usage: bubble notifications read <id|all>")
		}
		if args[1] == "all" {
			err = client.MarkRead(cfg, nil, true)
		} else {
			var ids []int64
			for _, a := range args[1:] {
				n, e := strconv.ParseInt(a, 10, 64)
				if e != nil {
					log.Fatalf("notifications read: %q is not an id", a)
				}
				ids = append(ids, n)
			}
			err = client.MarkRead(cfg, ids, false)
		}
	default:
		fmt.Fprint(os.Stderr, "usage: bubble notifications [read <id|all> | on | off]\n")
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("notifications: %v", err)
	}
}

func cmdTick(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Tick(cfg); err != nil {
		log.Fatalf("tick: %v", err)
	}
}

// cmdBirth creates a thread in a bubble. The server enforces the §3 birth rule
// (Brief with a Definition of Done, plus a Logbook unless --small).
func cmdBirth(args []string) {
	if len(args) < 1 {
		birthUsage()
		os.Exit(2)
	}
	id := args[0]
	fs := flag.NewFlagSet("birth", flag.ExitOnError)
	name := fs.String("name", "", "thread name (required)")
	brief := fs.String("brief", "", "Brief text (or use --brief-file)")
	briefFile := fs.String("brief-file", "", "read the Brief from a file")
	logbook := fs.String("logbook", "", "Logbook text (or use --logbook-file)")
	logbookFile := fs.String("logbook-file", "", "read the Logbook from a file")
	small := fs.Bool("small", false, "small thread — allow an empty Logbook (§3.2)")
	_ = fs.Parse(args[1:])

	readIf := func(inline, path, label string) string {
		if path == "" {
			return inline
		}
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("birth: reading %s: %v", label, err)
		}
		return string(b)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Birth(cfg, id, *name,
		readIf(*brief, *briefFile, "--brief-file"),
		readIf(*logbook, *logbookFile, "--logbook-file"), *small); err != nil {
		log.Fatalf("birth: %v", err)
	}
}

func birthUsage() {
	fmt.Fprint(os.Stderr, `bubble birth — create a thread in a bubble (enforces the §3 birth rule)

Usage:
  bubble birth <bubble-id> --name <name> \
      (--brief <text> | --brief-file <path>) \
      (--logbook <text> | --logbook-file <path> | --small)

The Brief must include a "Definition of Done". <bubble-id> is the short id from 'bubble ls'.

`)
}

// cmdBubble sets a bubble's §4 contract or opens/closes it (via the server).
func cmdBubble(args []string) {
	if len(args) < 1 {
		bubbleUsage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	sub := args[0]

	// `new` takes flags, not a positional id.
	if sub == "new" {
		fs := flag.NewFlagSet("bubble new", flag.ExitOnError)
		ws := fs.String("workspace", "", "<instance>:<project-id>")
		name := fs.String("name", "", "bubble name")
		outcome := fs.String("outcome", "", "§4 contract: what \"done\" looks like (optional)")
		owner := fs.String("owner", "", "§4 contract: who is accountable (optional)")
		_ = fs.Parse(args[1:])
		if *ws == "" || *name == "" {
			log.Fatal("bubble new: --workspace <instance>:<project> and --name are required")
		}
		inst, proj, found := strings.Cut(*ws, ":")
		if !found || inst == "" || proj == "" {
			log.Fatal("bubble new: --workspace must be <instance>:<project-id>")
		}
		if err := client.CreateBubble(cfg, domain.CreateBubbleRequest{
			Instance: inst, Project: proj, Name: *name, Outcome: *outcome, Owner: *owner,
		}); err != nil {
			log.Fatalf("bubble new: %v", err)
		}
		return
	}

	if len(args) < 2 {
		bubbleUsage()
		os.Exit(2)
	}
	id := args[1]

	switch sub {
	case "set":
		fs := flag.NewFlagSet("bubble set", flag.ExitOnError)
		outcome := fs.String("outcome", "", "what 'done' looks like")
		owner := fs.String("owner", "", "who is accountable now")
		closure := fs.String("closure", "", "the explicit close signal")
		_ = fs.Parse(args[2:])
		var in domain.ContractInput
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "outcome":
				v := *outcome
				in.Outcome = &v
			case "owner":
				v := *owner
				in.Owner = &v
			case "closure":
				v := *closure
				in.Closure = &v
			}
		})
		if in.Outcome == nil && in.Owner == nil && in.Closure == nil {
			log.Fatal("bubble set: provide at least one of --outcome / --owner / --closure")
		}
		if err := client.SetContract(cfg, id, in); err != nil {
			log.Fatalf("bubble set: %v", err)
		}
	case "rename":
		fs := flag.NewFlagSet("bubble rename", flag.ExitOnError)
		name := fs.String("name", "", "the new name (required)")
		_ = fs.Parse(args[2:])
		if *name == "" {
			log.Fatal("bubble rename: --name is required")
		}
		if err := client.RenameBubble(cfg, id, *name); err != nil {
			log.Fatalf("bubble rename: %v", err)
		}
	case "close":
		if err := client.Close(cfg, id); err != nil {
			log.Fatalf("bubble close: %v", err)
		}
	case "open":
		if err := client.Reopen(cfg, id); err != nil {
			log.Fatalf("bubble open: %v", err)
		}
	case "review":
		if err := client.Review(cfg, id); err != nil {
			log.Fatalf("bubble review: %v", err)
		}
	case "unreview":
		if err := client.Unreview(cfg, id); err != nil {
			log.Fatalf("bubble unreview: %v", err)
		}
	default:
		bubbleUsage()
		os.Exit(2)
	}
}

func bubbleUsage() {
	fmt.Fprint(os.Stderr, `bubble bubble — create a bubble, or set its contract (§4) / rename / open-close it

Usage:
  bubble bubble new --workspace <instance>:<project-id> --name <name>
  bubble bubble set <id> [--outcome <text>] [--owner <name>] [--closure <text>]
  bubble bubble rename <id> --name <name>
  bubble bubble close <id>   ·   open <id>
  bubble bubble review <id>  ·   unreview <id>

<id> is the short ID from 'bubble ls' (or any unique prefix of it).

`)
}

// cmdWorkspace creates a Plane project (our Workspace) with modules enabled.
func cmdWorkspace(args []string) {
	if len(args) < 1 {
		workspaceUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "new":
	case "list", "ls":
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		if err := client.ListWorkspaces(cfg); err != nil {
			log.Fatalf("workspace list: %v", err)
		}
		return
	case "rename":
		cmdWorkspaceRename(args[1:])
		return
	default:
		workspaceUsage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("workspace new", flag.ExitOnError)
	inst := fs.String("instance", "", "instance slug (required)")
	name := fs.String("name", "", "workspace name (required)")
	ident := fs.String("identifier", "", "Plane project identifier (auto-derived if omitted)")
	noCycles := fs.Bool("no-cycles", false, "disable Plane cycles (on by default — heat cadence)")
	noPages := fs.Bool("no-pages", false, "disable Plane pages (on by default)")
	views := fs.Bool("views", false, "also enable Plane views")
	intake := fs.Bool("intake", false, "also enable Plane intake")
	_ = fs.Parse(args[1:])
	if *inst == "" || *name == "" {
		log.Fatal("workspace new: --instance and --name are required")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.CreateWorkspace(cfg, domain.CreateWorkspaceRequest{
		Instance: *inst, Name: *name, Identifier: *ident,
		NoCycles: *noCycles, NoPages: *noPages, Views: *views, Intake: *intake,
	}); err != nil {
		log.Fatalf("workspace new: %v", err)
	}
}

// cmdWorkspaceRename retitles a Workspace. Renaming is not production — what a
// body of work is called is not what has been done — so no heat is earned.
func cmdWorkspaceRename(args []string) {
	fs := flag.NewFlagSet("workspace rename", flag.ExitOnError)
	id := fs.String("id", "", "<instance>:<project-id> (required)")
	name := fs.String("name", "", "the new name (required)")
	_ = fs.Parse(args)
	if *id == "" || *name == "" {
		log.Fatal("workspace rename: --id <instance>:<project-id> and --name are required")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.RenameWorkspace(cfg, *id, *name); err != nil {
		log.Fatalf("workspace rename: %v", err)
	}
}

// cmdPage reads and writes a workspace's documentation. Bodies move as markdown
// on stdin/stdout so they compose with the rest of a shell: a spec can be pulled
// out, edited in a real editor, and pushed back.
func cmdPage(args []string) {
	if len(args) < 1 {
		pageUsage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	switch args[0] {
	case "list", "ls":
		fs := flag.NewFlagSet("page list", flag.ExitOnError)
		ws := fs.String("workspace", "", "<instance>:<project-id> (required)")
		_ = fs.Parse(args[1:])
		if *ws == "" {
			log.Fatal("page list: --workspace <instance>:<project-id> is required")
		}
		if err := client.ListPages(cfg, *ws); err != nil {
			log.Fatalf("page list: %v", err)
		}
	case "read", "cat":
		if len(args) < 2 {
			log.Fatal("usage: bubble page read <instance>:<project>:<page>")
		}
		if err := client.ReadPage(cfg, args[1]); err != nil {
			log.Fatalf("page read: %v", err)
		}
	case "new":
		fs := flag.NewFlagSet("page new", flag.ExitOnError)
		ws := fs.String("workspace", "", "<instance>:<project-id> (required)")
		title := fs.String("title", "", "page title (required)")
		file := fs.String("file", "", "markdown file, or - for stdin")
		parent := fs.String("parent", "", "nest under this page id (optional; root if omitted)")
		_ = fs.Parse(args[1:])
		if *ws == "" || *title == "" {
			log.Fatal("page new: --workspace and --title are required")
		}
		if err := client.CreatePage(cfg, *ws, *title, readBody(*file), *parent); err != nil {
			log.Fatalf("page new: %v", err)
		}
	case "edit":
		fs := flag.NewFlagSet("page edit", flag.ExitOnError)
		id := fs.String("id", "", "<instance>:<project>:<page> (required)")
		title := fs.String("title", "", "new title (optional)")
		file := fs.String("file", "", "markdown file, or - for stdin — REPLACES the body")
		_ = fs.Parse(args[1:])
		if *id == "" {
			log.Fatal("page edit: --id is required")
		}
		var tp, bp *string
		if *title != "" {
			tp = title
		}
		if *file != "" {
			body := readBody(*file)
			bp = &body
		}
		if tp == nil && bp == nil {
			log.Fatal("page edit: pass --title, --file, or both")
		}
		if err := client.UpdatePage(cfg, *id, tp, bp); err != nil {
			log.Fatalf("page edit: %v", err)
		}
	default:
		pageUsage()
		os.Exit(2)
	}
}

// readBody reads a markdown body from a file, or from stdin when the name is
// "-", so a page can be piped in.
func readBody(name string) string {
	if name == "" {
		return ""
	}
	var b []byte
	var err error
	if name == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(name)
	}
	if err != nil {
		log.Fatalf("reading %s: %v", name, err)
	}
	return string(b)
}

func pageUsage() {
	fmt.Fprint(os.Stderr, `bubble page — a workspace's documentation (Plane project pages)

Usage:
  bubble page list --workspace <instance>:<project-id>
  bubble page read <instance>:<project>:<page>
  bubble page new  --workspace <instance>:<project-id> --title <t> [--file <f>|-]
  bubble page edit --id <instance>:<project>:<page> [--title <t>] [--file <f>|-]

Bodies are Markdown. read prints to stdout and --file - reads stdin, so:
  bubble page read <id> > spec.md && $EDITOR spec.md
  bubble page edit --id <id> --file spec.md

A page is reference material for the WHOLE workspace — a spec, a decision
record. Anything about one piece of work belongs in that thread's Brief or
Logbook. Pages earn no heat.

`)
}

func workspaceUsage() {
	fmt.Fprint(os.Stderr, `bubble workspace — a Plane project (our Workspace), with modules on

Usage:
  bubble workspace new --instance <slug> --name <name> [--identifier <ID>] \
                       [--no-cycles] [--no-pages] [--views] [--intake]
  bubble workspace list
  bubble workspace rename --id <instance>:<project-id> --name <name>

Modules, Cycles and Pages are enabled by default; Views/Intake are opt-in.
To destroy a workspace and everything in it: bubble delete workspace <id>.

`)
}

// tokenDisplay renders a stored key: full when reveal is set, otherwise masked
// to a recognizable fingerprint (prefix…suffix) that leaks nothing usable.
func tokenDisplay(token string, reveal bool) string {
	if token == "" {
		return "(none)"
	}
	if reveal {
		return token
	}
	if len(token) <= 18 {
		return "****"
	}
	return token[:14] + "…" + token[len(token)-4:]
}

// cmdUse switches the active credential profile, or lists profiles with no arg.
func cmdUse(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	fs := flag.NewFlagSet("use", flag.ExitOnError)
	tokens := fs.Bool("tokens", false, "show each profile's Plane API key (masked)")
	reveal := fs.Bool("reveal", false, "with --tokens, print the FULL key (careful: exposes secrets on screen)")
	_ = fs.Parse(args)
	rest := fs.Args()

	names := cfg.ProfileNames()

	if len(rest) == 0 {
		if len(names) == 0 {
			fmt.Println("no profiles yet — add one with `bubble init --name <name> --token <key>`")
			return
		}
		for _, n := range names {
			marker := "  "
			if n == cfg.Current {
				marker = "* "
			}
			p := cfg.Profiles[n]
			server := p.Server
			if server == "" {
				server = cfg.ServerURL + " (default)"
			}
			if *tokens {
				fmt.Printf("%s%-12s %-24s → %s\n", marker, n, tokenDisplay(p.Token, *reveal), server)
			} else {
				fmt.Printf("%s%-12s → %s\n", marker, n, server)
			}
		}
		if *tokens && !*reveal {
			fmt.Println("\n(keys masked — add --reveal to print them in full)")
		}
		return
	}

	name := rest[0]
	if _, ok := cfg.Profiles[name]; !ok {
		if len(names) == 0 {
			log.Fatalf("use: no profile named %q — add one with `bubble init --name %s --token <key>`", name, name)
		}
		log.Fatalf("use: no profile named %q (have: %s)", name, strings.Join(names, ", "))
	}
	cfg.Current = name
	if err := config.Save(cfg); err != nil {
		log.Fatalf("use: %v", err)
	}
	fmt.Printf("switched to profile %q → %s\n", name, cfg.ActiveServer())
}

// cmdAdmin runs service-admin (godmode) operations. It authenticates with
// $BUBBLE_ADMIN_TOKEN if set, else the active profile (for admin-email humans).
func cmdAdmin(args []string) {
	if len(args) < 1 {
		adminUsage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	token := os.Getenv("BUBBLE_ADMIN_TOKEN")
	if token == "" {
		token = cfg.ActiveToken()
	}
	switch args[0] {
	case "instances":
		err = client.AdminInstances(cfg, token)
	case "bubbles":
		err = client.AdminBubbles(cfg, token)
	case "stats":
		err = client.AdminStats(cfg, token)
	case "refresh":
		err = client.AdminRefresh(cfg, token)
	case "tick":
		err = client.AdminTick(cfg, token)
	case "kiosk":
		err = cmdAdminKiosk(cfg, token, args[1:])
	case "tuning":
		err = cmdAdminTuning(cfg, token, args[1:])
	case "outbox":
		err = client.AdminOutbox(cfg, token, args[1:])
	case "sync", "sync-diff", "sync-fidelity", "sync-backfill", "sync-rebuild":
		if len(args) < 2 {
			err = fmt.Errorf("usage: bubble admin %s <instance>", args[0])
			break
		}
		err = client.AdminSync(cfg, token, args[0], args[1])
	case "autostate":
		if len(args) < 3 {
			err = fmt.Errorf("usage: bubble admin autostate <instance> <on|off>")
			break
		}
		switch args[2] {
		case "on", "true", "yes":
			err = client.AdminAutoState(cfg, token, args[1], true)
		case "off", "false", "no":
			err = client.AdminAutoState(cfg, token, args[1], false)
		default:
			err = fmt.Errorf("expected on or off, got %q", args[2])
		}
	default:
		adminUsage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("admin: %v", err)
	}
}

func cmdAdminKiosk(cfg config.Config, token string, args []string) error {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, "usage: bubble admin kiosk [ls | new <instance> [name...] | rm <token>]\n")
		os.Exit(2)
	}
	switch args[0] {
	case "ls", "list":
		return client.AdminKioskList(cfg, token)
	case "new", "add":
		if len(args) < 2 {
			return fmt.Errorf("usage: bubble admin kiosk new <instance> [name...]")
		}
		name := strings.Join(args[2:], " ")
		return client.AdminKioskNew(cfg, token, args[1], name)
	case "rm", "revoke", "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: bubble admin kiosk rm <token>")
		}
		return client.AdminKioskRevoke(cfg, token, args[1])
	default:
		return fmt.Errorf("unknown kiosk subcommand %q", args[0])
	}
}

// cmdAdminTuning shows or edits the buoyancy calibration — the thresholds that
// govern how bubbles AND threads move between bands (THREAD-LIFECYCLE.md).
func cmdAdminTuning(cfg config.Config, token string, args []string) error {
	if len(args) == 0 {
		return client.AdminTuning(cfg, token)
	}
	switch args[0] {
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("usage: bubble admin tuning set <key>=<value> [<key>=<value>...]")
		}
		return client.AdminTuningSet(cfg, token, args[1:])
	case "reset":
		return client.AdminTuningReset(cfg, token)
	default:
		return fmt.Errorf("unknown tuning subcommand %q (want: set, reset)", args[0])
	}
}

func adminUsage() {
	fmt.Fprint(os.Stderr, `bubble admin — service-admin (godmode) operations

Auth: $BUBBLE_ADMIN_TOKEN (break-glass), or your active profile if your email is
an admin email.

Usage:
  bubble admin instances     list all instances (across orgs)
  bubble admin bubbles       list bubbles across ALL instances
  bubble admin stats         server health snapshot
  bubble admin refresh       flush server caches
  bubble admin tick          force a cooling sweep now
  bubble admin kiosk ls              list read-only display tokens
  bubble admin kiosk new <inst> [n]  mint a kiosk token for an instance
  bubble admin kiosk rm <token>      revoke a kiosk token
  bubble admin tuning                show the buoyancy calibration
  bubble admin tuning set k=v [k=v]  tweak lifecycle thresholds
  bubble admin tuning reset          restore the stock calibration
  bubble admin autostate <inst> on   write derived levels back to Plane (Phase B)
  bubble admin autostate <inst> off  stop writing to Plane (default)
  bubble admin sync <inst>           mirror census + cursor (no Plane calls)
  bubble admin sync-diff <inst>      compare the mirror against a live fetch
  bubble admin sync-fidelity <inst>  what a write would do to mirrored bodies
  bubble admin sync-backfill <inst>  force a complete re-walk of one instance
  bubble admin sync-rebuild <inst>   drop the local mirror and rebuild it
  bubble admin outbox                writes that have not reached Plane
  bubble admin outbox drop <id>      clear one stuck entry

`)
}

// cmdReset purges all local Bubble Work state (config + database), returning to
// a clean slate. It only removes files Bubble owns — never a whole directory —
// and confirms first unless --force is given.
func cmdReset(args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	force := fs.Bool("force", false, "skip the confirmation prompt")
	fs.BoolVar(force, "y", false, "skip the confirmation prompt (shorthand)")
	_ = fs.Parse(args)

	home, err := config.Home()
	if err != nil {
		log.Fatalf("reset: %v", err)
	}
	cfgPath, _ := config.Path()
	dbPath, _ := config.DBPath()

	var targets []string
	for _, p := range []string{cfgPath, dbPath} {
		if _, err := os.Stat(p); err == nil {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		fmt.Printf("nothing to reset — %s is already clean\n", home)
		return
	}

	if !*force {
		fmt.Printf("This deletes Bubble Work state in %s:\n", home)
		for _, p := range targets {
			fmt.Printf("  - %s\n", filepath.Base(p))
		}
		fmt.Print("Including every registered instance and its API key. Continue? [y/N]: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			fmt.Println("aborted")
			return
		}
	}

	for _, p := range targets {
		if err := os.Remove(p); err != nil {
			log.Fatalf("reset: %v", err)
		}
	}
	// Drop the home dir too, but only if nothing else is in it.
	if entries, err := os.ReadDir(home); err == nil && len(entries) == 0 {
		_ = os.Remove(home)
	}
	fmt.Println("reset complete — start fresh with `bubble instance add`")
}

// openLocalStore opens the server's SQLite DB directly. Admin commands
// (instance, member) operate on it locally on the server host.
func openLocalStore() *store.Store {
	dbPath, err := config.DBPath()
	if err != nil {
		log.Fatalf("db path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	return st
}

// cmdInstance manages the federated Plane instances (local DB, server host).
func cmdInstance(args []string) {
	if len(args) < 1 {
		instanceUsage()
		os.Exit(2)
	}
	st := openLocalStore()
	defer st.Close()

	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("instance add", flag.ExitOnError)
		slug := fs.String("slug", "", "short id, e.g. ayetec (required)")
		name := fs.String("name", "", "display name (optional)")
		url := fs.String("url", "", "Plane base URL, e.g. https://plane.ayetec.space (required)")
		key := fs.String("key", "", "Plane API key (required)")
		ws := fs.String("workspace", "", "Plane workspace slug (required)")
		proj := fs.String("project", "", "Plane project id to pin (optional; omit for the whole workspace)")
		force := fs.Bool("force", false, "overwrite an existing instance with the same slug")
		noVerify := fs.Bool("no-verify", false, "skip the live connectivity check against Plane")
		_ = fs.Parse(args[1:])
		if *slug == "" || *url == "" || *key == "" || *ws == "" {
			log.Fatal("instance add: --slug, --url, --key and --workspace are required (--project is optional)")
		}
		exists, err := st.InstanceExists(*slug)
		if err != nil {
			log.Fatalf("instance add: %v", err)
		}
		if exists && !*force {
			log.Fatalf("instance add: an instance named %q already exists — choose a different --slug, or pass --force to overwrite it (this replaces its URL, key and workspace)", *slug)
		}
		if !*noVerify {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			u, err := plane.New(*url, *key, *ws, *proj).Verify(ctx)
			cancel()
			if err != nil {
				log.Fatalf("instance add: connectivity check failed: %v\n(use --no-verify to register anyway)", err)
			}
			fmt.Printf("verified — authenticated as %s\n", u.Email)
		}
		if err := st.AddInstance(domain.Instance{
			Slug: *slug, Name: *name, BaseURL: *url, APIKey: *key, Workspace: *ws, Project: *proj,
		}); err != nil {
			log.Fatalf("instance add: %v", err)
		}
		verb := "registered"
		if exists {
			verb = "overwrote"
		}
		if *proj == "" {
			fmt.Printf("%s instance %s → %s (workspace %s, ALL projects)\n", verb, *slug, *url, *ws)
		} else {
			fmt.Printf("%s instance %s → %s (workspace %s, project %s)\n", verb, *slug, *url, *ws, *proj)
		}
	case "list":
		is, err := st.ListInstances()
		if err != nil {
			log.Fatalf("instance list: %v", err)
		}
		if len(is) == 0 {
			fmt.Println("no instances yet — add one with `bubble instance add`")
			return
		}
		for _, i := range is {
			proj := i.Project
			if proj == "" {
				proj = "(all projects)"
			}
			fmt.Printf("%-12s  %-30s  workspace=%s  project=%s\n", i.Slug, i.BaseURL, i.Workspace, proj)
		}
	case "webhook":
		fs := flag.NewFlagSet("instance webhook", flag.ExitOnError)
		secret := fs.String("secret", "", "the webhook secret from Plane (required)")
		if len(args) < 2 {
			log.Fatal("usage: bubble instance webhook <slug> --secret <secret>")
		}
		slug := args[1]
		_ = fs.Parse(args[2:])
		if *secret == "" {
			log.Fatal("instance webhook: --secret is required (create the webhook in Plane, then paste its secret)")
		}
		ok, err := st.SetWebhookSecret(slug, *secret)
		if err != nil {
			log.Fatalf("instance webhook: %v", err)
		}
		if !ok {
			log.Fatalf("instance webhook: no such instance %q", slug)
		}
		fmt.Printf("webhook secret set for %s\n", slug)
		fmt.Printf("register a Plane webhook pointing at:  <your-public-url>/webhooks/plane/%s\n", slug)
	case "remove":
		if len(args) < 2 {
			log.Fatal("usage: bubble instance remove <slug>")
		}
		ok, err := st.RemoveInstance(args[1])
		if err != nil {
			log.Fatalf("instance remove: %v", err)
		}
		if !ok {
			fmt.Println("no such instance")
			return
		}
		fmt.Println("removed (and revoked its grants)")
	default:
		instanceUsage()
		os.Exit(2)
	}
}

func instanceUsage() {
	fmt.Fprint(os.Stderr, `bubble instance — manage Plane instances (run on the server host)

Usage:
  bubble instance add --slug <slug> --url <base-url> --key <api-key> \
                      --workspace <ws> [--project <project-id>] [--name <name>] [--force] [--no-verify]
                      (omit --project to federate the whole workspace;
                       --force overwrites an existing slug, otherwise add errors;
                       add verifies connectivity to Plane unless --no-verify)
  bubble instance list
  bubble instance webhook <slug> --secret <secret>   (enable real-time sync)
  bubble instance remove <slug>

`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "listen address (overrides config)")
	verbose := fs.Bool("verbose", false, "also print logs to the console (they always go to the log file — see `bubble attach`)")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	// Precedence: --addr flag, then $PORT (services-platform convention), then config.
	if *addr != "" {
		cfg.Addr = *addr
	} else if p := os.Getenv("PORT"); p != "" {
		cfg.Addr = ":" + p
	}

	dbPath, err := config.DBPath()
	if err != nil {
		log.Fatalf("db path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	// Refuse to start a second server; point the user at stop/attach instead.
	pidPath, _ := config.PidPath()
	if pidPath != "" {
		if data, err := os.ReadFile(pidPath); err == nil {
			if old, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil &&
				old != os.Getpid() && processAlive(old) {
				log.Fatalf("a bubble server is already running (pid %d) — `bubble stop` it, or `bubble attach` to watch it", old)
			}
		}
	}

	// Logs always go to a file (so `bubble attach` can show them), but the console
	// stays quiet by default — pass --verbose to also print them here.
	if lp, err := config.LogPath(); err == nil {
		if f, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
			if *verbose {
				log.SetOutput(io.MultiWriter(os.Stderr, f))
			} else {
				log.SetOutput(f)
			}
			defer f.Close()
		}
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	if insts, _ := st.ListInstances(); len(insts) == 0 {
		log.Printf("warning: no Plane instances configured — add one with `bubble instance add`")
	}

	srv := server.New(st, cfg.Cycle())
	srv.SetAdmin(os.Getenv("BUBBLE_ADMIN_TOKEN"), cfg.AdminEmails)
	srv.SetMCPHosts(cfg.MCPHosts)
	if len(cfg.MCPHosts) > 0 {
		log.Printf("mcp hosts: %s", strings.Join(cfg.MCPHosts, ", "))
	}
	if os.Getenv("BUBBLE_ADMIN_TOKEN") != "" || len(cfg.AdminEmails) > 0 {
		log.Printf("service-admin enabled (token=%v, %d admin email(s))", os.Getenv("BUBBLE_ADMIN_TOKEN") != "", len(cfg.AdminEmails))
	}
	// Background refresher keeps a warm, full snapshot so reads never fetch Plane
	// on the request path (fast loads, no partial "popping").
	go srv.RunRefresher(context.Background())
	if iv := cfg.TickInterval(); iv > 0 {
		go srv.RunTicker(context.Background(), iv)
		log.Printf("cooling sweep every %s", iv)
	}
	// The Plane mirror runs in SHADOW during Phase 1: it fills sqlite but nothing
	// reads from it yet, so `bubble admin sync-diff` can prove it agrees with
	// Plane before Phase 2 points the board at it. It runs in the background rate
	// lane, so it yields to page loads rather than competing with them.
	// Retry writes that could not reach Plane (Phase 5). Auto-state moves only:
	// comment drafts carry no credential and are re-sent by their author.
	go srv.RunOutbox(context.Background())
	if sy := srv.Syncer(); sy != nil {
		go sy.Run(context.Background(), srv.Instances)
		log.Printf("plane mirror syncing (delta %s, full reconcile %s) — shadow mode, nothing reads it yet",
			planesync.DeltaInterval, planesync.FullInterval)
	}

	// Record our pid so `bubble stop` can find us; clean it up on exit.
	if pidPath != "" {
		_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o644)
		defer os.Remove(pidPath)
	}

	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv.Handler()}
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
		<-sigc
		log.Println("shutting down…")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
	}()

	if !*verbose {
		fmt.Printf("bubble-work %s listening on %s — logs quiet (use `bubble attach`, or --verbose)\n", version, cfg.Addr)
	}
	log.Printf("bubble-work %s listening on %s (REST /api, MCP %s/mcp)", version, cfg.Addr, cfg.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
	log.Println("stopped")
}

// processAlive reports whether a process with pid exists (signal 0 probe).
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func cmdStop(args []string) {
	pidPath, err := config.PidPath()
	if err != nil {
		log.Fatalf("pid path: %v", err)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no bubble server appears to be running (no pid file).")
			return
		}
		log.Fatalf("read pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		log.Fatalf("bad pid file %q: %v", pidPath, err)
	}
	if !processAlive(pid) {
		_ = os.Remove(pidPath)
		fmt.Printf("server (pid %d) was not running — cleaned up stale pid file.\n", pid)
		return
	}
	p, _ := os.FindProcess(pid)
	if err := p.Signal(syscall.SIGTERM); err != nil {
		log.Fatalf("stop pid %d: %v", pid, err)
	}
	fmt.Printf("stopped bubble server (pid %d).\n", pid)
}

// cmdAttach follows the running server's log (Ctrl-C detaches; the server keeps
// running). Shows the current session's output so far, then streams new lines.
func cmdAttach(args []string) {
	lp, err := config.LogPath()
	if err != nil {
		log.Fatalf("log path: %v", err)
	}
	f, err := os.Open(lp)
	if err != nil {
		if os.IsNotExist(err) {
			log.Fatal("no server log found — start one with `bubble serve`.")
		}
		log.Fatalf("open log: %v", err)
	}
	defer f.Close()

	if pidPath, err := config.PidPath(); err == nil {
		if data, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && processAlive(pid) {
				fmt.Fprintf(os.Stderr, "— attached to bubble server (pid %d); Ctrl-C to detach —\n", pid)
			} else {
				fmt.Fprintln(os.Stderr, "— server not running; showing last session's log —")
			}
		}
	}

	buf := make([]byte, 4096)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if err == io.EOF {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			return
		}
	}
}

func cmdLs(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Ls(cfg); err != nil {
		log.Fatalf("ls: %v", err)
	}
}

func cmdHeat(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: bubble heat <bubble-id>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Heat(cfg, args[0]); err != nil {
		log.Fatalf("heat: %v", err)
	}
}

func cmdShow(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: bubble show <bubble-id>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Show(cfg, args[0]); err != nil {
		log.Fatalf("show: %v", err)
	}
}

func cmdThread(args []string) {
	// Accept --comments before or after the id (the stdlib flag parser would
	// otherwise stop at the first positional arg).
	comments := false
	var id string
	for _, a := range args {
		switch a {
		case "--comments", "-comments", "-c":
			comments = true
		default:
			if id == "" && !strings.HasPrefix(a, "-") {
				id = a
			}
		}
	}
	if id == "" {
		log.Fatal("usage: bubble thread <id> [--comments]   (id from `bubble show`)")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.Thread(cfg, id, comments); err != nil {
		log.Fatalf("thread: %v", err)
	}
}

// cmdLogbook rewrites a thread's Logbook. "-" reads it from stdin, which is how
// a plan of any real length gets in without shell quoting.
func cmdLogbook(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble logbook <id> <text...>   (or '-' to read stdin)")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	body := strings.Join(args[1:], " ")
	if body == "-" {
		b, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			log.Fatalf("logbook: reading stdin: %v", rerr)
		}
		body = string(b)
	}
	if err := client.UpdateThread(cfg, args[0], domain.ThreadEdit{Logbook: &body}); err != nil {
		log.Fatalf("logbook: %v", err)
	}
}

// A Definition of Done is a Logbook's sibling: same shape, same write path,
// different section. It is what closes a Brief (§ thread birth rule).
func cmdDoD(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble dod <id> <text...>   (or '-' to read stdin)")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	body := strings.Join(args[1:], " ")
	if body == "-" {
		b, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			log.Fatalf("dod: reading stdin: %v", rerr)
		}
		body = string(b)
	}
	if err := client.UpdateThread(cfg, args[0], domain.ThreadEdit{DoD: &body}); err != nil {
		log.Fatalf("dod: %v", err)
	}
}

// cmdSection adds, rewrites or removes one named section of a thread's document.
//
// A thread page is ONE document, so a new part of it is a new `## ` section.
// This addresses it by heading instead of making you quote its text back.
func cmdSection(args []string) {
	fs := flag.NewFlagSet("section", flag.ExitOnError)
	id := fs.String("id", "", "thread id (required)")
	title := fs.String("title", "", "section heading (required)")
	file := fs.String("file", "", "markdown file, or - for stdin")
	del := fs.Bool("delete", false, "remove the section, heading and all")
	_ = fs.Parse(args)
	if *id == "" || *title == "" {
		log.Fatal("section: --id and --title are required")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	e := domain.SectionEdit{Title: *title, Delete: *del}
	if !*del {
		body := readBody(*file)
		e.Markdown = &body
	}
	if err := client.UpdateThread(cfg, *id, domain.ThreadEdit{
		Sections: []domain.SectionEdit{e},
	}); err != nil {
		log.Fatalf("section: %v", err)
	}
}

// cmdRename retitles a thread or a revision. Both are Plane work items, so both
// take the same path.
func cmdRename(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble rename <id> <new title...>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	title := strings.Join(args[1:], " ")
	if err := client.UpdateThread(cfg, args[0], domain.ThreadEdit{Title: &title}); err != nil {
		log.Fatalf("rename: %v", err)
	}
}

// cmdMove re-homes a thread. Reversible by construction — the same call the
// other way puts it back — so it does not prompt.
func cmdMove(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble move <thread-id> <bubble-id>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.MoveThread(cfg, args[0], args[1]); err != nil {
		log.Fatalf("move: %v", err)
	}
}

// cmdDelete removes a bubble, a thread or one artifact — from PLANE, which is
// the system of record. There is no undo, so it asks first unless -y is given.
// cmdDone marks a thread finished. The server refuses when the Definition of Done
// still has unticked items — the DoD is the authority on "done", and --force is
// the deliberate override for a DoD that turned out to be wrong.
func cmdDone(args []string) {
	fs := flag.NewFlagSet("done", flag.ExitOnError)
	force := fs.Bool("force", false, "complete even though the Definition of Done is unmet")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		log.Fatal("usage: bubble done <thread-id> [--force]")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.CompleteThread(cfg, fs.Arg(0), *force); err != nil {
		log.Fatalf("done: %v", err)
	}
}

// cmdReopen puts a finished thread back to work.
func cmdReopen(args []string) {
	if len(args) < 1 {
		log.Fatal("usage: bubble reopen <thread-id>")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := client.ReopenThread(cfg, args[0]); err != nil {
		log.Fatalf("reopen: %v", err)
	}
}

func cmdDelete(args []string) {
	yes := false
	var rest []string
	for _, a := range args {
		if a == "-y" || a == "--yes" {
			yes = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) < 2 {
		log.Fatal("usage: bubble delete <workspace|bubble|thread|artifact> <id> [logbook|dod|brief] [-y]")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	kind, id := rest[0], rest[1]
	var path, what, warning string
	switch kind {
	case "workspace":
		path, what = "/api/workspaces/"+url.PathEscape(id), "workspace "+id
		warning = "EVERY bubble, thread, artifact and comment inside it goes too.\n" +
			"This is the most destructive thing you can do here, and Plane keeps no copy."
	case "bubble":
		path, what = "/api/bubbles/"+url.PathEscape(id), "bubble "+id
		warning = "Its threads survive in Plane but will belong to no bubble, so they leave the board.\n" +
			"Closing a bubble (bubble bubble close) keeps the record of what was done."
	case "thread":
		path, what = "/api/threads/"+url.PathEscape(id), "thread "+id
		warning = "Its Brief, Logbook, comments and revisions go with it."
	case "artifact", "region":
		if len(rest) < 3 {
			log.Fatal("usage: bubble delete artifact <thread-id> <logbook|dod|brief> [-y]")
		}
		path = "/api/threads/" + url.PathEscape(id) + "/regions/" + url.PathEscape(rest[2])
		what = rest[2] + " of " + id
		warning = "The thread survives; only this section is removed."
	default:
		log.Fatalf("delete what? workspace, bubble, thread or artifact — not %q", kind)
	}

	if !yes {
		fmt.Printf("Delete %s from Plane? This cannot be undone.\n%s\n\nType 'yes' to continue: ", what, warning)
		var answer string
		fmt.Scanln(&answer)
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			fmt.Println("left alone")
			return
		}
	}
	if err := client.Delete(cfg, path, what); err != nil {
		log.Fatalf("delete: %v", err)
	}
}

// cmdTodo ticks one checklist item. The text is REQUIRED, not decoration: it is
// what stops a stale index from ticking somebody else's todo.
func cmdTodo(args []string) {
	if len(args) < 3 {
		log.Fatal("usage: bubble todo <id> <n> <done|open> <text...>  [--dod]")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	n, err := strconv.Atoi(args[1])
	if err != nil {
		log.Fatalf("todo: %q is not a position", args[1])
	}
	var done bool
	switch strings.ToLower(args[2]) {
	case "done", "x", "tick":
		done = true
	case "open", "untick", "-":
		done = false
	default:
		log.Fatalf("todo: say 'done' or 'open', not %q", args[2])
	}
	region := "logbook"
	var words []string
	for _, a := range args[3:] {
		if a == "--dod" {
			region = "dod"
			continue
		}
		words = append(words, a)
	}
	if err := client.ToggleTodo(cfg, args[0], region, n, strings.Join(words, " "), done); err != nil {
		log.Fatalf("todo: %v", err)
	}
}

func cmdRevision(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble revision <id> <title> [body...|-]")
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	body := strings.Join(args[2:], " ")
	if body == "" || body == "-" {
		b, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			log.Fatalf("revision: reading stdin: %v", rerr)
		}
		body = string(b)
	}
	if err := client.AddRevision(cfg, args[0], args[1], body); err != nil {
		log.Fatalf("revision: %v", err)
	}
}

func cmdComment(args []string) {
	if len(args) < 2 {
		log.Fatal("usage: bubble comment <id> <text...> | --retry <draft> | --discard <draft>")
	}
	id := args[0]
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	// A comment that could not reach Plane is kept as a draft rather than lost
	// (docs/PLANE-SYNC.md Phase 5). These re-send or throw one away.
	if args[1] == "--retry" || args[1] == "--discard" {
		if len(args) < 3 {
			log.Fatalf("usage: bubble comment %s %s <draft-id>", id, args[1])
		}
		draft, perr := strconv.ParseInt(args[2], 10, 64)
		if perr != nil {
			log.Fatalf("comment: %q is not a draft id", args[2])
		}
		if args[1] == "--retry" {
			err = client.RetryDraft(cfg, id, draft)
		} else {
			err = client.DiscardDraft(cfg, id, draft)
		}
		if err != nil {
			log.Fatalf("comment: %v", err)
		}
		return
	}
	if err := client.Comment(cfg, id, strings.Join(args[1:], " ")); err != nil {
		log.Fatalf("comment: %v", err)
	}
}

func cmdInit(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	server := fs.String("server", "", "server base URL — pinned to the profile when --name is given, else the active profile's (or the global default)")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "server listen address")
	fs.IntVar(&cfg.CycleHours, "cycle-hours", cfg.CycleHours, "heat-window pulse length in hours")
	name := fs.String("name", "", "profile name to store the token/server under (e.g. a workspace); switches to it")
	token := fs.String("token", "", "your Plane API key")
	_ = fs.Parse(args)

	switch {
	case *name != "":
		// Create/update a named profile with its own token + server, and activate
		// it. Empty flags leave existing values intact (so you can set just one).
		cfg.UpsertProfile(*name, *token, *server)
	case *token != "":
		cfg.Token = *token // legacy single credential (no profile)
		if *server != "" {
			cfg.ServerURL = *server
		}
	case *server != "":
		// No name/token: retarget the ACTIVE profile's server (not the global
		// default), so switching a server can't leak across profiles.
		if cfg.Current != "" {
			cfg.UpsertProfile(cfg.Current, "", *server)
		} else {
			cfg.ServerURL = *server
		}
	}

	if err := config.Save(cfg); err != nil {
		log.Fatalf("save: %v", err)
	}
	if *name != "" {
		fmt.Printf("saved profile %q (server %s) and set it active\n", *name, cfg.ActiveServer())
	}
	p, _ := config.Path()
	fmt.Printf("config saved to %s\n", p)
}
