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
	case "use":
		cmdUse(os.Args[2:])
	case "instance":
		cmdInstance(os.Args[2:])
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
  bubble heat <bubble-id>          explain a bubble's temperature
  bubble whoami                    show the identity resolved from your credential
  bubble use [name]                switch active credential profile (no arg: list)
  bubble instance add|list|remove  manage Plane instances (run on the server host)
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
	if *addr != "" {
		cfg.Addr = *addr
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
