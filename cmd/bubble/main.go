// Command bubble is the single Bubble Work binary. `bubble serve` runs the brain
// (server + MCP); the other subcommands are the thin client (§9).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AngelMaldonado/bubble-work/internal/client"
	"github.com/AngelMaldonado/bubble-work/internal/config"
	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
	"github.com/AngelMaldonado/bubble-work/internal/server"
	"github.com/AngelMaldonado/bubble-work/internal/store"
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
	case "ls":
		cmdLs(os.Args[2:])
	case "heat":
		cmdHeat(os.Args[2:])
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
  bubble ls                        list bubbles, hottest first (buoyancy view)
  bubble heat <id>                 explain a bubble's temperature (id from 'ls')
  bubble whoami                    show the identity resolved from your credential
  bubble notifications             your inbox of cooling/dormant alerts (alias: inbox)
  bubble notifications on|off      opt in/out of notifications
  bubble notifications read <id|all>  mark notifications read
  bubble tick                      sweep now for cooling bubbles
  bubble use [name]                switch active credential profile (no arg: list)
  bubble workspace new [flags]     create a Plane project (modules on) — our Workspace
  bubble birth <id> [flags]        create a thread in a bubble (needs Brief + Logbook)
  bubble bubble new|set|close|open create a bubble or set its contract (§4)
  bubble instance add|list|remove  manage Plane instances (run on the server host)
  bubble admin <cmd>               service-admin ops (godmode; needs admin token/email)
  bubble init [flags]              configure server URL + a credential profile
  bubble reset [--force]           purge all local state and start from scratch

Everyone authenticates with a Plane API key — identity, role and instance scope
are discovered from Plane. Store one key per workspace/identity as a named
profile (`+"`bubble init --name cuby --token <key>`"+`) and switch between them
with `+"`bubble use cuby`"+`. An agent impersonates a human by using that human's key.

`)
}

func cmdWhoami(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if cfg.Current != "" {
		fmt.Printf("profile   : %s\n", cfg.Current)
	}
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
		_ = fs.Parse(args[1:])
		if *ws == "" || *name == "" {
			log.Fatal("bubble new: --workspace <instance>:<project> and --name are required")
		}
		inst, proj, found := strings.Cut(*ws, ":")
		if !found || inst == "" || proj == "" {
			log.Fatal("bubble new: --workspace must be <instance>:<project-id>")
		}
		if err := client.CreateBubble(cfg, domain.CreateBubbleRequest{Instance: inst, Project: proj, Name: *name}); err != nil {
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
	case "close":
		if err := client.Close(cfg, id); err != nil {
			log.Fatalf("bubble close: %v", err)
		}
	case "open":
		if err := client.Reopen(cfg, id); err != nil {
			log.Fatalf("bubble open: %v", err)
		}
	default:
		bubbleUsage()
		os.Exit(2)
	}
}

func bubbleUsage() {
	fmt.Fprint(os.Stderr, `bubble bubble — create a bubble, or set its contract (§4) / open-close it

Usage:
  bubble bubble new --workspace <instance>:<project-id> --name <name>
  bubble bubble set <id> [--outcome <text>] [--owner <name>] [--closure <text>]
  bubble bubble close <id>
  bubble bubble open  <id>

<id> is the short ID from 'bubble ls' (or any unique prefix of it).

`)
}

// cmdWorkspace creates a Plane project (our Workspace) with modules enabled.
func cmdWorkspace(args []string) {
	if len(args) < 1 || args[0] != "new" {
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

func workspaceUsage() {
	fmt.Fprint(os.Stderr, `bubble workspace — create a Plane project (our Workspace) with modules on

Usage:
  bubble workspace new --instance <slug> --name <name> [--identifier <ID>] \
                       [--no-cycles] [--no-pages] [--views] [--intake]

Modules, Cycles and Pages are enabled by default; Views/Intake are opt-in.

`)
}

// cmdUse switches the active credential profile, or lists profiles with no arg.
func cmdUse(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	names := cfg.ProfileNames()

	if len(args) == 0 {
		if len(names) == 0 {
			fmt.Println("no profiles yet — add one with `bubble init --name <name> --token <key>`")
			return
		}
		for _, n := range names {
			marker := "  "
			if n == cfg.Current {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, n)
		}
		return
	}

	name := args[0]
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
	fmt.Printf("switched to profile %q — run `bubble whoami` to confirm\n", name)
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
	default:
		adminUsage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatalf("admin: %v", err)
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
	if os.Getenv("BUBBLE_ADMIN_TOKEN") != "" || len(cfg.AdminEmails) > 0 {
		log.Printf("service-admin enabled (token=%v, %d admin email(s))", os.Getenv("BUBBLE_ADMIN_TOKEN") != "", len(cfg.AdminEmails))
	}
	if iv := cfg.TickInterval(); iv > 0 {
		go srv.RunTicker(context.Background(), iv)
		log.Printf("cooling sweep every %s", iv)
	}
	log.Printf("bubble-work %s listening on %s (REST /api, MCP %s/mcp)", version, cfg.Addr, cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, srv.Handler()))
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

func cmdInit(args []string) {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.StringVar(&cfg.ServerURL, "server", cfg.ServerURL, "server base URL (client)")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "server listen address")
	fs.IntVar(&cfg.CycleHours, "cycle-hours", cfg.CycleHours, "heat-window pulse length in hours")
	name := fs.String("name", "", "profile name to store the token under (e.g. a workspace); switches to it")
	token := fs.String("token", "", "your Plane API key")
	_ = fs.Parse(args)

	switch {
	case *token != "" && *name != "":
		cfg.SetProfile(*name, *token) // store under a profile and activate it
	case *token != "":
		cfg.Token = *token // legacy single credential (no profile)
	}

	if err := config.Save(cfg); err != nil {
		log.Fatalf("save: %v", err)
	}
	if *token != "" && *name != "" {
		fmt.Printf("saved profile %q and set it active\n", *name)
	}
	p, _ := config.Path()
	fmt.Printf("config saved to %s\n", p)
}
