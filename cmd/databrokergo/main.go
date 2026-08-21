package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/nunchimangchi-dev/unbrokerrdd/internal/config"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/dashboard"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/db"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/orchestrator"
	"github.com/nunchimangchi-dev/unbrokerrdd/internal/agent"
	"github.com/nunchimangchi-dev/unbrokerrdd/strategies"
)

const version = "0.1.0"
const dbPath = "databrokergo.db"

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {

	// ── serve ──────────────────────────────────────────────────────────
	// Starts the dashboard. Loads real broker state from SQLite if the DB
	// exists; otherwise seeds it from the built-in broker registry.
	case "serve":
		addr := "127.0.0.1:8080"
		fmt.Printf("DATABROKER.GO v%s\n", version)
		fmt.Printf("Dashboard: http://localhost:8080\n")
		fmt.Printf("Bound to:  %s (loopback only)\n", addr)

		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		if err := dashboard.Serve(addr, store); err != nil {
			log.Fatal(err)
		}

	// ── run ────────────────────────────────────────────────────────────
	// Dispatches agents for one strategy batch.
	//
	// Usage:
	//   databrokergo run --strategy 1 --dry-run
	//   databrokergo run --strategy 1
	//   databrokergo run --strategy 1 --limit 3
	case "run":
		args := parseFlags(os.Args[2:])

		strategyNum, _ := strconv.Atoi(args["strategy"])
		if strategyNum == 0 {
			fmt.Fprintln(os.Stderr, "usage: databrokergo run --strategy <1-6> [--dry-run] [--limit N]")
			os.Exit(1)
		}
		_, dryRun := args["dry-run"]
		limit, _ := strconv.Atoi(args["limit"])

		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("load config: %v", err)
		}

		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		mode := "LIVE"
		if dryRun {
			mode = "DRY_RUN"
		}
		fmt.Printf("DATABROKER.GO v%s — run strategy %d [%s]\n", version, strategyNum, mode)
		if !dryRun {
			redacted := cfg.Redacted()
			fmt.Printf("Subject: %s | State: %s\n", redacted.SubjectName, cfg.SubjectState)
		}

		agents := map[int]agent.Agent{
			1: strategies.NewStrategy1(),
			// 2-6: added in future phases
		}

		runner := orchestrator.New(store, cfg, agents, func(u orchestrator.StatusUpdate) {
			icon := statusIcon(u.Status)
			dry := ""
			if u.DryRun {
				dry = " [dry-run]"
			}
			fmt.Printf("  %s %s → %s%s\n", icon, u.BrokerID, u.Status, dry)
			if u.Notes != "" {
				fmt.Printf("     ↳ %s\n", u.Notes)
			}
		})

		delay := 5 * time.Second
		if dryRun {
			delay = 500 * time.Millisecond // faster in dry-run
		}

		if err := runner.RunBatch(context.Background(), orchestrator.BatchConfig{
			Strategy: strategyNum,
			DryRun:   dryRun,
			Limit:    limit,
			Delay:    delay,
		}); err != nil {
			log.Fatalf("batch failed: %v", err)
		}
		fmt.Println("Batch complete.")

	// ── status ─────────────────────────────────────────────────────────
	case "status":
		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		stats, err := store.Stats()
		if err != nil {
			log.Fatalf("stats: %v", err)
		}

		total := 0
		for _, n := range stats {
			total += n
		}
		fmt.Printf("DATABROKER.GO v%s — broker status\n\n", version)
		fmt.Printf("  %-14s %d\n", "TOTAL", total)
		for _, s := range []db.Status{db.StatusPending, db.StatusSuccess, db.StatusFailed, db.StatusInProgress, db.StatusSkipped, db.StatusManual} {
			if n := stats[s]; n > 0 || s == db.StatusPending {
				fmt.Printf("  %s %-12s %d\n", statusIcon(s), s, n)
			}
		}

	// ── reset ──────────────────────────────────────────────────────────
	case "reset":
		if len(os.Args) < 4 || os.Args[2] != "--broker" {
			fmt.Fprintln(os.Stderr, "usage: databrokergo reset --broker <broker-id>")
			os.Exit(1)
		}
		id := os.Args[3]
		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()
		if err := store.Reset(id); err != nil {
			log.Fatalf("reset: %v", err)
		}
		fmt.Printf("reset %s → pending\n", id)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "commands: serve | run | status | reset")
		os.Exit(1)
	}
}

// openStore opens the SQLite DB and seeds all brokers (idempotently).
func openStore() (*db.Store, error) {
	store, err := db.New(dbPath)
	if err != nil {
		return nil, err
	}
	if err := store.Seed(dashboard.AllBrokers()); err != nil {
		return nil, fmt.Errorf("seed brokers: %v", err)
	}
	return store, nil
}

func statusIcon(s db.Status) string {
	switch s {
	case db.StatusPending:
		return "○"
	case db.StatusInProgress:
		return "⚡"
	case db.StatusSuccess:
		return "✓"
	case db.StatusFailed:
		return "✗"
	case db.StatusSkipped:
		return "⏭"
	case db.StatusManual:
		return "⚑"
	default:
		return "?"
	}
}

// parseFlags parses --key or --key value style args into a map.
func parseFlags(args []string) map[string]string {
	out := make(map[string]string)
	for i := 0; i < len(args); i++ {
		k := args[i]
		if !startsWithDash(k) {
			continue
		}
		k = trimDashes(k)
		if i+1 < len(args) && !startsWithDash(args[i+1]) {
			out[k] = args[i+1]
			i++
		} else {
			out[k] = ""
		}
	}
	return out
}

func startsWithDash(s string) bool { return len(s) > 0 && s[0] == '-' }
func trimDashes(s string) string {
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	return s
}
