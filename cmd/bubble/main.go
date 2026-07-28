// Command bubble is the single Bubble Work binary. `bubble serve` runs the brain
// (server + MCP); the other subcommands are the thin client (§9).
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AngelMaldonado/bubble-work/internal/client"
	"github.com/AngelMaldonado/bubble-work/internal/config"
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
	case "init":
		cmdInit(os.Args[2:])
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
  bubble init [flags]              configure server URL + Plane connection
  bubble version

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

	pl := plane.New(cfg.PlaneBaseURL, cfg.PlaneAPIKey, cfg.PlaneWorkspace, cfg.PlaneProject)
	if !pl.Configured() {
		log.Printf("warning: Plane not configured — serving an empty world. Run `bubble init`.")
	}

	srv := server.New(st, pl, cfg.Cycle())
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
	fs.StringVar(&cfg.PlaneBaseURL, "plane-url", cfg.PlaneBaseURL, "Plane API base URL")
	fs.StringVar(&cfg.PlaneAPIKey, "plane-key", cfg.PlaneAPIKey, "Plane API key")
	fs.StringVar(&cfg.PlaneWorkspace, "plane-workspace", cfg.PlaneWorkspace, "Plane workspace slug")
	fs.StringVar(&cfg.PlaneProject, "plane-project", cfg.PlaneProject, "Plane project id (our Workspace)")
	fs.IntVar(&cfg.CycleHours, "cycle-hours", cfg.CycleHours, "heat-window pulse length in hours")
	fs.StringVar(&cfg.Token, "token", cfg.Token, "this member's server credential")
	_ = fs.Parse(args)

	if err := config.Save(cfg); err != nil {
		log.Fatalf("save: %v", err)
	}
	p, _ := config.Path()
	fmt.Printf("saved config to %s\n", p)
}
