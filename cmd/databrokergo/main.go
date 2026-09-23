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
		// Defaults to loopback-only. Set DATABROKERGO_BIND to listen on a
		// real interface (e.g. behind a reverse proxy/tunnel that isn't on
		// the same host) - never bind a public interface directly, since
		// the dashboard has no login of its own.
		addr := "127.0.0.1:8080"
		if v := os.Getenv("DATABROKERGO_BIND"); v != "" {
			addr = v
		}
		fmt.Printf("DATABROKER.GO v%s\n", version)
		fmt.Printf("Dashboard bound to: %s\n", addr)

		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		if prof, err := store.GetSubjectProfile(); err == nil && prof != nil {
			cfg.SubjectName = prof.Name
			cfg.SubjectEmail = prof.Email
			cfg.SubjectState = prof.State
		}

		agents := map[int]agent.Agent{
			1: strategies.NewStrategy1(),
			2: strategies.NewStrategy2(),
			// 3-6: added in future phases
		}

		if err := dashboard.Serve(addr, store, cfg, agents); err != nil {
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

		if prof, err := store.GetSubjectProfile(); err == nil && prof != nil {
			cfg.SubjectName = prof.Name
			cfg.SubjectEmail = prof.Email
			cfg.SubjectState = prof.State
		}

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
			2: strategies.NewStrategy2(),
			// 3-6: added in future phases
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

		registry := dashboard.AllBrokers()
		orphans, err := store.OrphanedBrokerIDs(registry)
		if err != nil {
			log.Fatalf("orphan check: %v", err)
		}
		completion, err := store.CompletionStats()
		if err != nil {
			log.Fatalf("completion stats: %v", err)
		}
		blockers, err := store.BlockerStats()
		if err != nil {
			log.Fatalf("blocker stats: %v", err)
		}
		presence, err := store.PresenceStats()
		if err != nil {
			log.Fatalf("presence stats: %v", err)
		}

		fmt.Printf("DATABROKER.GO v%s — broker status\n\n", version)

		// Registry (what the code declares) vs database (live state). These
		// drifting apart silently is what let a removed broker keep inflating
		// counts for days — report both rather than picking one to trust.
		fmt.Printf("  REGISTRY       %d targets declared in code\n", len(registry))
		fmt.Printf("  DATABASE       %d rows\n", total)
		if len(orphans) > 0 {
			fmt.Printf("  ⚠ ORPHANS      %d in database but not in registry: %v\n", len(orphans), orphans)
		}
		fmt.Println()

		for _, s := range []db.Status{db.StatusPending, db.StatusSuccess, db.StatusFailed, db.StatusInProgress, db.StatusSkipped, db.StatusManual} {
			if n := stats[s]; n > 0 || s == db.StatusPending {
				fmt.Printf("  %s %-12s %d\n", statusIcon(s), s, n)
			}
		}

		// The number the project actually exists to move: how many of those
		// successes the automation earned on its own.
		if stats[db.StatusSuccess] > 0 {
			fmt.Printf("\n  OF %d SUCCESSES:\n", stats[db.StatusSuccess])
			fmt.Printf("    %-18s %d\n", "autonomous", completion[db.CompletionAutonomous])
			fmt.Printf("    %-18s %d\n", "human-completed", completion[db.CompletionHuman])
			if n := completion[db.CompletionNone]; n > 0 {
				fmt.Printf("    %-18s %d  (unrecorded — predates completion tracking)\n", "unattributed", n)
			}
		}

		if len(blockers) > 0 {
			fmt.Printf("\n  BLOCKED, BY REASON:\n")
			for _, b := range []db.BlockerType{
				db.BlockerBotDefended, db.BlockerNeedsProfileURL, db.BlockerMissingField,
				db.BlockerNoMechanism, db.BlockerDeadSite, db.BlockerCoveredByOther, db.BlockerUnbuilt,
			} {
				if n := blockers[b]; n > 0 {
					fmt.Printf("    %-18s %d\n", b, n)
				}
			}
		}

		// Exposure, not progress. Every claim this project makes is measured
		// against a denominator, and an unchecked registry is not one — it is a
		// list of sites the subject may not even be on. Until these are
		// checked, "95 brokers" describes the tool's ambition, not anyone's
		// actual exposure.
		fmt.Printf("\n  ACTUAL EXPOSURE (presence checks):\n")
		fmt.Printf("    %-18s %d\n", "confirmed listed", presence[db.PresencePresent])
		fmt.Printf("    %-18s %d\n", "confirmed absent", presence[db.PresenceAbsent])
		fmt.Printf("    %-18s %d\n", "undetermined", presence[db.PresenceUndetermined])
		fmt.Printf("    %-18s %d\n", "never checked", presence[db.PresenceUnknown])
		if presence[db.PresenceUnknown] > 0 {
			fmt.Printf("    → %d of %d targets have never been checked for a record at all;\n",
				presence[db.PresenceUnknown], total)
			fmt.Printf("      any coverage percentage over %d is not yet a real number.\n", total)
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

	// ── blocker ────────────────────────────────────────────────────────
	case "blocker":
		args := parseFlags(os.Args[2:])
		id := args["broker"]
		blockerType := args["type"]
		if id == "" || blockerType == "" {
			fmt.Fprintln(os.Stderr, "usage: databrokergo blocker --broker <broker-id> --type <blocker-type> [--covered-by <other-broker-id>]")
			fmt.Fprintln(os.Stderr, "types: dead_site | bot_defended | needs_profile_url | no_mechanism | missing_field | covered_by_other | unbuilt | none")
			os.Exit(1)
		}
		bt := db.BlockerType(blockerType)
		if blockerType == "none" {
			bt = db.BlockerNone
		}
		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()
		if err := store.SetBlocker(id, bt, args["covered-by"]); err != nil {
			log.Fatalf("set blocker: %v", err)
		}
		fmt.Printf("%s → blocker_type=%s", id, blockerType)
		if args["covered-by"] != "" {
			fmt.Printf(" covered_by=%s", args["covered-by"])
		}
		fmt.Println()

	// ── set-profile-url ───────────────────────────────────────────────
	// The subject's own real listing URL is PII-adjacent (a specific
	// identifier tied to their actual record) - meant to be run directly
	// by the subject on the machine holding the real data, same as
	// editing .env, not relayed through anything else.
	case "set-profile-url":
		args := parseFlags(os.Args[2:])
		id := args["broker"]
		url := args["url"]
		if id == "" || url == "" {
			fmt.Fprintln(os.Stderr, "usage: databrokergo set-profile-url --broker <broker-id> --url <listing-url>")
			os.Exit(1)
		}
		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()
		if err := store.SetProfileURL(id, url); err != nil {
			log.Fatalf("set profile url: %v", err)
		}
		fmt.Printf("%s → profile_url set\n", id)

	// ── completed ─────────────────────────────────────────────────────
	// Records a removal a human performed by hand (because a site blocked
	// automation, or the flow needed a CAPTCHA solved, or an emailed
	// verification link clicked). Deliberately cannot record "autonomous" -
	// only Settle can, and only by the agent actually succeeding on a live
	// run. Marking work as automated that a person did would defeat the
	// entire point of tracking this.
	case "completed":
		args := parseFlags(os.Args[2:])
		id := args["broker"]
		if id == "" {
			fmt.Fprintln(os.Stderr, "usage: databrokergo completed --broker <broker-id> [--note \"...\"]")
			fmt.Fprintln(os.Stderr, "marks a broker as success, completed by a human (not by the automation)")
			os.Exit(1)
		}
		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		note := args["note"]
		if note == "" {
			note = "Completed manually by the subject."
		}
		if err := store.Settle(id, db.StatusSuccess, false, "human_completed", note, ""); err != nil {
			log.Fatalf("settle: %v", err)
		}
		// Settle would have stamped this autonomous (live run + success), so
		// correct it immediately - a person did this, not the code.
		if err := store.SetCompletionMethod(id, db.CompletionHuman); err != nil {
			log.Fatalf("set completion method: %v", err)
		}
		fmt.Printf("%s → success (human-completed)\n", id)

	// ── probe ─────────────────────────────────────────────────────────
	// Read-only reconnaissance: loads a page in the same headless browser
	// the strategies use and reports what IT sees. Never fills, never
	// submits. Use this before classifying a site - judging from a plain
	// HTTP fetch, a handler timeout, or your own Chrome has produced a
	// wrong blocker_type every time it's been tried.
	case "probe":
		args := parseFlags(os.Args[2:])
		target := args["url"]
		if target == "" && args["broker"] != "" {
			for _, b := range dashboard.AllBrokers() {
				if b.ID == args["broker"] {
					target = "https://" + b.URL + "/"
					break
				}
			}
			if target == "" {
				log.Fatalf("no broker with id %q in the registry", args["broker"])
			}
		}
		if target == "" {
			fmt.Fprintln(os.Stderr, "usage: databrokergo probe --url <url>   (or --broker <broker-id>)")
			os.Exit(1)
		}

		// Not gated on the allowlist: probing is read-only, human-invoked,
		// and its whole purpose is evaluating sites not yet committed to.
		// The allowlist guards automated submission paths, which this isn't.
		if err := agent.ValidateURL(target); err != nil {
			fmt.Printf("note: %s is not in the broker allowlist — fine for recon, but it must be added before any handler can navigate there\n\n", target)
		}

		fmt.Printf("probing %s (headless, read-only)…\n\n", target)
		res, probeErr := agent.Probe(context.Background(), target)
		if probeErr != nil {
			res.NavErr = probeErr.Error()
			fmt.Printf("  NAVIGATION FAILED: %v\n", probeErr)
		}

		fmt.Printf("  final url   %s\n", res.URL)
		fmt.Printf("  title       %q\n", res.Title)
		fmt.Printf("  challenge   %v\n", res.Challenge)
		fmt.Printf("  captcha     %v %s\n", res.Captcha, res.CaptchaKind)

		if len(res.Inputs) == 0 {
			fmt.Printf("\n  no form controls rendered\n")
		} else {
			fmt.Printf("\n  form controls (%d):\n", len(res.Inputs))
			for _, in := range res.Inputs {
				vis := "visible"
				if !in.Visible {
					vis = "HIDDEN "
				}
				fmt.Printf("    %s %-10s name=%-22q id=%-20q %s\n", vis, in.Type, in.Name, in.ID, in.Placeholder)
				if in.ClickableLabel != "" {
					fmt.Printf("              ↳ zero-size: click %s instead, not the input\n", in.ClickableLabel)
				}
			}
		}

		if snippet := res.BodySnippet; snippet != "" {
			fmt.Printf("\n  body starts: %.160s\n", snippet)
		}
		if probeErr != nil {
			fmt.Printf("\n  → NO CONCLUSION about this site: the browser never completed the\n")
			fmt.Printf("    request, so nothing here is evidence about the site itself.\n")
			fmt.Printf("    Check the local browser before classifying anything.\n")
		} else if b := res.SuggestedBlocker(); b != "" {
			fmt.Printf("\n  → suggests blocker_type=%s\n", b)
		} else {
			fmt.Printf("\n  → no block detected; if a handler still fails here, it's the selectors, not the site\n")
		}

	case "presence":
		args := parseFlags(os.Args[2:])

		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("config: %v", err)
		}
		if cfg.SubjectName == "" {
			log.Fatal("SUBJECT_NAME is not set; a presence check has nothing to search for")
		}

		// Ad-hoc mode: verify a candidate search template live before it is
		// ever committed to SearchTemplates. --name supplies a stand-in, so a
		// template can be proven against a site without the subject's real
		// name entering a terminal, a log, or a pasted transcript. This is the
		// gate the map's doc comment refers to: nothing goes in untested.
		if _, verify := args["verify-template"]; verify && args["url"] != "" {
			tmpl := args["url"]
			fmt.Printf("verifying search template against %s\n  %s\n\n", args["site"], tmpl)
			fmt.Println("  running a control experiment: one name that must be found,")
			fmt.Println("  one that cannot be. A template only earns trust by passing both.")
			fmt.Println()

			v, vErr := agent.VerifyTemplate(context.Background(), cfg.AnthropicKey, args["site"], tmpl, args["control"], args["decoy"])
			if vErr != nil {
				log.Fatalf("%v", vErr)
			}
			fmt.Printf("  control  %-13s %s\n", v.Control.Finding, v.Control.Evidence)
			fmt.Printf("  decoy    %-13s %s\n", v.Decoy.Finding, v.Decoy.Evidence)
			if v.Trusted {
				fmt.Printf("\n  ✓ TRUSTED — %s\n", v.Reasoning)
				fmt.Println("    safe to add to agent.SearchTemplates")
			} else {
				fmt.Printf("\n  ✗ NOT TRUSTED — %s\n", v.Reasoning)
				fmt.Println("    do not add this to agent.SearchTemplates; its negatives would be false all-clears")
			}
			return
		}

		if tmpl := args["url"]; tmpl != "" {
			name := args["name"]
			if name == "" {
				name = cfg.SubjectName
			}
			searchURL, buildErr := agent.BuildSearchURL(tmpl, name)
			if buildErr != nil {
				log.Fatalf("%v", buildErr)
			}
			fmt.Printf("ad-hoc presence check (read-only)\n  %s\n\n", agent.RedactURL(searchURL))
			res, checkErr := agent.CheckPresence(context.Background(), cfg.AnthropicKey, args["site"], searchURL, name)
			if checkErr != nil {
				log.Fatalf("%v", checkErr)
			}
			fmt.Printf("  finding     %s\n", res.Finding)
			fmt.Printf("  evidence    %s\n", res.Evidence)
			fmt.Printf("  challenge   %v\n", res.Challenge)
			fmt.Printf("  page text   %d chars\n", res.PageChars)
			fmt.Println("\n  nothing written to the database (ad-hoc mode)")
			return
		}

		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		all, err := store.GetAll()
		if err != nil {
			log.Fatalf("read brokers: %v", err)
		}

		// Select targets: one broker, one strategy, or every broker that has a
		// verified search template and has not been checked yet.
		var targets []db.Broker
		switch {
		case args["broker"] != "":
			for _, b := range all {
				if b.ID == args["broker"] {
					targets = append(targets, b)
				}
			}
			if len(targets) == 0 {
				log.Fatalf("no broker with id %q", args["broker"])
			}
		case args["strategy"] != "":
			n, convErr := strconv.Atoi(args["strategy"])
			if convErr != nil {
				log.Fatalf("--strategy must be a number: %v", convErr)
			}
			for _, b := range all {
				if b.Strategy == n {
					targets = append(targets, b)
				}
			}
		default:
			for _, b := range all {
				if _, ok := agent.SearchTemplateFor(b.ID); ok && b.Presence == db.PresenceUnknown {
					targets = append(targets, b)
				}
			}
		}

		if limit := args["limit"]; limit != "" {
			if n, convErr := strconv.Atoi(limit); convErr == nil && n < len(targets) {
				targets = targets[:n]
			}
		}
		if len(targets) == 0 {
			fmt.Println("nothing to check: no matching brokers with a verified search template")
			return
		}

		_, dryRun := args["dry-run"]
		fmt.Printf("presence check — %d target(s), read-only, searching as %s\n\n",
			len(targets), cfg.Redacted().SubjectName)

		var present, absent, undet, noTemplate int
		for _, b := range targets {
			tmpl, ok := agent.SearchTemplateFor(b.ID)
			if !ok {
				noTemplate++
				fmt.Printf("  —  %-24s no verified search template; not checked\n", b.ID)
				continue
			}
			searchURL, buildErr := agent.BuildSearchURL(tmpl, cfg.SubjectName)
			if buildErr != nil {
				noTemplate++
				fmt.Printf("  !  %-24s %v\n", b.ID, buildErr)
				continue
			}

			res, checkErr := agent.CheckPresence(context.Background(), cfg.AnthropicKey, b.Name, searchURL, cfg.SubjectName)
			if checkErr != nil {
				fmt.Printf("  !  %-24s %v\n", b.ID, checkErr)
				continue
			}

			icon := "?"
			switch res.Finding {
			case agent.FindingPresent:
				icon, present = "●", present+1
			case agent.FindingAbsent:
				icon, absent = "○", absent+1
			default:
				icon, undet = "?", undet+1
			}
			fmt.Printf("  %s  %-24s %-13s %s\n", icon, b.ID, res.Finding, res.Evidence)
			fmt.Printf("     %s\n", agent.RedactURL(res.SearchURL))

			if dryRun {
				continue
			}
			if err := store.SetPresence(b.ID, db.Presence(res.Finding), res.Evidence); err != nil {
				fmt.Printf("     ! could not record: %v\n", err)
			}
		}

		fmt.Printf("\n  present %d · absent %d · undetermined %d · no template %d\n",
			present, absent, undet, noTemplate)
		if dryRun {
			fmt.Println("  (dry run — nothing written to the database)")
		}
		if absent > 0 {
			fmt.Println("\n  absent sites move to 'skipped'. That is not a removal and is never")
			fmt.Println("  counted as one — it shrinks the denominator, honestly.")
		}

	case "reach":
		args := parseFlags(os.Args[2:])
		_, apply := args["apply"]

		store, err := openStore()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}
		defer store.Close()

		all, err := store.GetAll()
		if err != nil {
			log.Fatalf("read brokers: %v", err)
		}

		var targets []db.Broker
		for _, b := range all {
			if args["broker"] != "" && b.ID != args["broker"] {
				continue
			}
			// Already-classified dead sites need no re-checking, and live
			// successes are self-evidently reachable.
			if b.BlockerType == db.BlockerDeadSite || b.Status == db.StatusSuccess {
				continue
			}
			targets = append(targets, b)
		}
		if n, convErr := strconv.Atoi(args["limit"]); convErr == nil && n < len(targets) {
			targets = targets[:n]
		}
		if len(targets) == 0 {
			fmt.Println("nothing to check")
			return
		}

		fmt.Printf("reachability sweep — %d domain(s), read-only\n", len(targets))
		fmt.Printf("two independent checks per domain: DNS, then a real browser load\n\n")

		var dead, alive, odd int
		for _, b := range targets {
			r := agent.CheckReachability(context.Background(), b.URL)
			isDead, reason := r.Verdict()
			switch {
			case isDead:
				dead++
				fmt.Printf("  ✗ %-26s DEAD  %s\n", b.ID, reason)
				if apply {
					if err := store.SetBlocker(b.ID, db.BlockerDeadSite, ""); err != nil {
						fmt.Printf("      ! could not record: %v\n", err)
					}
				}
			case reason != "":
				odd++
				fmt.Printf("  ? %-26s %s\n", b.ID, reason)
			default:
				alive++
				fmt.Printf("  ✓ %-26s alive  %.60q\n", b.ID, r.Title)
			}
		}

		fmt.Printf("\n  alive %d · dead %d · needs a look %d\n", alive, dead, odd)
		if !apply && dead > 0 {
			fmt.Println("  (nothing written — re-run with --apply to record blocker_type=dead_site)")
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "commands: serve | run | status | reset | blocker | set-profile-url | completed | probe | presence | reach")
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
