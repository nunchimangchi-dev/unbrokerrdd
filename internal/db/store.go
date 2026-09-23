// Package db manages broker state in a local SQLite database.
// The database is the source of truth — the dashboard reads from it,
// and agents write to it. It never leaves the local machine.
package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGo required
)

// Status mirrors the frontend status values exactly.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusSuccess    Status = "success"
	StatusFailed     Status = "failed"
	StatusSkipped    Status = "skipped"
	StatusManual     Status = "manual"
)

// BlockerType explains WHY a broker sits at manual (or is otherwise stuck),
// so a future session can triage the registry without re-investigating from
// scratch. Distinct from Status: a broker can be "manual" for five genuinely
// different reasons, and only some of them are worth ever retrying.
type BlockerType string

const (
	BlockerNone            BlockerType = ""                  // not yet classified, or not blocked
	BlockerDeadSite        BlockerType = "dead_site"          // domain gone/parked/seized - nothing to do, ever
	BlockerBotDefended     BlockerType = "bot_defended"       // active anti-automation defense (CAPTCHA, WAF 403 on plain reads, etc.) - do not attempt evasion
	BlockerNeedsProfileURL BlockerType = "needs_profile_url"  // requires a search-and-select-your-listing step; needs the ProfileURL mechanism
	BlockerNoMechanism     BlockerType = "no_mechanism"       // no self-serve opt-out found anywhere on the site
	BlockerMissingField    BlockerType = "missing_field"      // requires PII the tool deliberately doesn't collect (e.g. street address, phone) - a product decision, not a code fix
	BlockerCoveredByOther  BlockerType = "covered_by_other"   // resolved as a side effect of another broker's submission - see CoveredBy
	BlockerUnbuilt         BlockerType = "unbuilt"            // straightforward site, just no handler written yet
)

// CompletionMethod records HOW a broker reached StatusSuccess - whether the
// automation actually did it, or a human did it by hand.
//
// This exists because the tool could not previously answer its own central
// question. A `success` row looked identical whether chromedp completed it
// unattended or the subject clicked through the form themselves, which made
// "how much of this is really automated?" a matter of reading prose notes
// and remembering. Recording it as data makes automation coverage a number
// you can query and improve against, rather than a claim you have to defend.
type CompletionMethod string

const (
	CompletionNone      CompletionMethod = ""                // nothing was completed (pending/manual/skipped/dead)
	CompletionAutonomous CompletionMethod = "autonomous"     // the agent ran end to end on a live run and the outcome validated - no human in the loop
	CompletionHuman     CompletionMethod = "human_completed" // a person performed the actual submission, however the tool assisted
)

// Presence records whether the subject actually appears on a broker at all.
//
// This is deliberately a separate axis from Status and CompletionMethod. A site
// the subject was never listed on is not a removal, and must never be counted
// as one - but it is also not a failure, and it is the single most common
// honest outcome across business directories and niche profile sites. Keeping
// it in its own column means "how many sites list me" and "how many did the
// tool get me off" stay two different numbers that cannot be confused for each
// other.
type Presence string

const (
	PresenceUnknown      Presence = ""             // never checked
	PresencePresent      Presence = "present"      // subject appears in search results
	PresenceAbsent       Presence = "absent"       // search ran cleanly, no record of the subject
	PresenceUndetermined Presence = "undetermined" // check ran but could not be trusted either way
)

// Broker is one row in the brokers table.
type Broker struct {
	ID               string
	Name             string
	Strategy         int
	URL              string
	Status           Status
	AttemptCount     int
	LastAttemptAt    *time.Time
	ConfirmationURL  string
	Notes            string
	BlockerType      BlockerType
	CoveredBy        string // broker ID whose submission also resolves this one, when BlockerType == covered_by_other
	ProfileURL       string // subject's own listing URL, for sites requiring search-and-select-your-record before opt-out (see BlockerNeedsProfileURL)
	CompletionMethod CompletionMethod
	Presence         Presence
	PresenceCheckedAt *time.Time
}

// Store wraps the SQLite connection and exposes broker operations.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if adminEmail := os.Getenv("UNBROKERRDD_ADMIN_EMAIL"); adminEmail != "" {
		if err := s.BootstrapAllowedUsers(adminEmail); err != nil {
			db.Close()
			return nil, fmt.Errorf("bootstrap admin user: %w", err)
		}
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS brokers (
			id                  TEXT PRIMARY KEY,
			name                TEXT NOT NULL,
			strategy            INTEGER NOT NULL,
			url                 TEXT NOT NULL,
			status              TEXT NOT NULL DEFAULT 'pending',
			attempt_count       INTEGER NOT NULL DEFAULT 0,
			last_attempt_at     TIMESTAMP,
			confirmation_url    TEXT,
			notes               TEXT,
			blocker_type        TEXT NOT NULL DEFAULT '',
			covered_by          TEXT,
			profile_url         TEXT,
			completion_method   TEXT NOT NULL DEFAULT '',
			presence            TEXT NOT NULL DEFAULT '',
			presence_checked_at TIMESTAMP,
			created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS attempts (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			broker_id   TEXT NOT NULL REFERENCES brokers(id),
			status      TEXT NOT NULL,
			dry_run     INTEGER NOT NULL DEFAULT 0,
			result      TEXT,
			error       TEXT,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS allowed_users (
			email TEXT PRIMARY KEY,
			role  TEXT NOT NULL CHECK(role IN ('viewer','admin'))
		);

		CREATE TABLE IF NOT EXISTS subject_profile (
			id    INTEGER PRIMARY KEY CHECK (id = 1),
			name  TEXT NOT NULL,
			email TEXT NOT NULL,
			state TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}
	if err := s.addColumnIfMissing("brokers", "blocker_type", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("brokers", "covered_by", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("brokers", "profile_url", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("brokers", "completion_method", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing("brokers", "presence", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return s.addColumnIfMissing("brokers", "presence_checked_at", "TIMESTAMP")
}

// addColumnIfMissing upgrades an existing database created before a column
// was added to the CREATE TABLE statement above (e.g. the already-deployed
// prod database) - CREATE TABLE IF NOT EXISTS only helps fresh installs.
func (s *Store) addColumnIfMissing(table, column, ddl string) error {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if existing[column] {
		return nil
	}
	_, err = s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl))
	if err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

// Seed inserts all brokers as 'pending' using INSERT OR IGNORE.
// Existing rows (any status) are never touched — this is the idempotency foundation.
func (s *Store) Seed(brokers []Broker) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO brokers (id, name, strategy, url, status)
		VALUES (?, ?, ?, ?, 'pending')
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, b := range brokers {
		if _, err := stmt.Exec(b.ID, b.Name, b.Strategy, b.URL); err != nil {
			return fmt.Errorf("seed broker %s: %w", b.ID, err)
		}
	}
	return tx.Commit()
}

// CanDispatch is the idempotency guard.
// Returns true ONLY if the broker's current status is 'pending'.
// Any other status (in_progress, success, failed, skipped, manual) returns false.
// The orchestrator MUST call this before dispatching any agent.
func (s *Store) CanDispatch(id string) (bool, error) {
	var status string
	err := s.db.QueryRow(`SELECT status FROM brokers WHERE id = ?`, id).Scan(&status)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("broker %q not found in database", id)
	}
	if err != nil {
		return false, err
	}
	return Status(status) == StatusPending, nil
}

// SetInProgress marks a broker as in_progress.
// Returns an error if CanDispatch would return false (idempotency enforcement).
func (s *Store) SetInProgress(id string) error {
	ok, err := s.CanDispatch(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("broker %q is not pending — skipping (idempotency guard)", id)
	}
	_, err = s.db.Exec(`
		UPDATE brokers SET status = 'in_progress', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, id,
	)
	return err
}

// Settle records the final outcome of an agent run.
// Always logs an attempt record regardless of outcome.
//
// A live (non-dry-run) run that lands on StatusSuccess is recorded as
// CompletionAutonomous - by definition the agent got there itself, with no
// human in the loop. Nothing else sets that value; human-completed work is
// marked explicitly via SetCompletionMethod, because the tool can't observe
// it happening. That asymmetry is the point: autonomous successes can only
// be earned by the code actually working.
func (s *Store) Settle(id string, status Status, dryRun bool, result, notes, confirmURL string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	completion := ""
	if !dryRun && status == StatusSuccess {
		completion = string(CompletionAutonomous)
	}

	_, err = tx.Exec(`
		UPDATE brokers SET
			status            = ?,
			attempt_count     = attempt_count + 1,
			last_attempt_at   = CURRENT_TIMESTAMP,
			confirmation_url  = COALESCE(NULLIF(?, ''), confirmation_url),
			notes             = COALESCE(NULLIF(?, ''), notes),
			completion_method = COALESCE(NULLIF(?, ''), completion_method),
			updated_at        = CURRENT_TIMESTAMP
		WHERE id = ?`,
		string(status), confirmURL, notes, completion, id,
	)
	if err != nil {
		return fmt.Errorf("settle broker %s: %w", id, err)
	}

	dryRunInt := 0
	if dryRun {
		dryRunInt = 1
	}
	_, err = tx.Exec(`
		INSERT INTO attempts (broker_id, status, dry_run, result)
		VALUES (?, ?, ?, ?)`,
		id, string(status), dryRunInt, result,
	)
	if err != nil {
		return fmt.Errorf("log attempt for %s: %w", id, err)
	}

	return tx.Commit()
}

// GetAll returns every broker ordered by strategy then name.
func (s *Store) GetAll() ([]Broker, error) {
	rows, err := s.db.Query(`
		SELECT id, name, strategy, url, status, attempt_count,
		       last_attempt_at, COALESCE(confirmation_url,''), COALESCE(notes,''),
		       blocker_type, COALESCE(covered_by,''), COALESCE(profile_url,''), completion_method,
		       presence, presence_checked_at
		FROM brokers ORDER BY strategy, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBrokers(rows)
}

// GetPendingByStrategy returns all pending brokers for a given strategy number.
func (s *Store) GetPendingByStrategy(strategy int) ([]Broker, error) {
	rows, err := s.db.Query(`
		SELECT id, name, strategy, url, status, attempt_count,
		       last_attempt_at, COALESCE(confirmation_url,''), COALESCE(notes,''),
		       blocker_type, COALESCE(covered_by,''), COALESCE(profile_url,''), completion_method,
		       presence, presence_checked_at
		FROM brokers
		WHERE strategy = ? AND status = 'pending'
		ORDER BY name`,
		strategy,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBrokers(rows)
}

// SetBlocker records why a broker is stuck (or clears it by passing
// BlockerNone), independent of Status - a broker can be "manual" for
// several genuinely different reasons, and only classifying which one
// makes the registry queryable instead of requiring re-investigation
// from scratch every time. coveredByID is only meaningful when blocker
// is BlockerCoveredByOther; pass "" otherwise.
func (s *Store) SetBlocker(id string, blocker BlockerType, coveredByID string) error {
	var coveredBy any
	if coveredByID != "" {
		coveredBy = coveredByID
	}
	_, err := s.db.Exec(`
		UPDATE brokers SET blocker_type = ?, covered_by = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		string(blocker), coveredBy, id,
	)
	return err
}

// SetProfileURL records the subject's own listing URL for a site that
// requires search-and-select-your-record before opt-out (BlockerNeedsProfileURL).
// Does not change Status or BlockerType - the caller still needs a Strategy 2
// handler wired up to actually use it; this just supplies the one input the
// generic navigate/fill/submit/validate pattern needs for these sites.
func (s *Store) SetProfileURL(id, url string) error {
	_, err := s.db.Exec(`
		UPDATE brokers SET profile_url = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		url, id,
	)
	return err
}

// SetCompletionMethod records how a broker was actually completed. Only
// ever needed for CompletionHuman - CompletionAutonomous is set by Settle
// when the agent earns it, and cannot be claimed by hand here without
// misrepresenting what the automation did.
func (s *Store) SetCompletionMethod(id string, method CompletionMethod) error {
	_, err := s.db.Exec(`
		UPDATE brokers SET completion_method = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		string(method), id,
	)
	return err
}

// SetPresence records what a presence check concluded about a broker.
//
// It deliberately does NOT write to the attempts log. A presence check never
// submits anything and never touches an opt-out form, so counting it as an
// attempt would trip the 48h cooldown that exists to pace real submissions -
// the same mistake manual-routed attempts caused before they were excluded
// from LastLiveAttemptAt. Looking at a site is not attempting it.
//
// When the subject is absent the broker is moved to 'skipped', which is what
// that status has always meant ("nothing to do - no matching record"). Nothing
// here can ever set a success or a completion_method: establishing that there
// was no record is not a removal, and must not be counted as one.
func (s *Store) SetPresence(id string, p Presence, evidence string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		UPDATE brokers SET
			presence            = ?,
			presence_checked_at = CURRENT_TIMESTAMP,
			notes               = COALESCE(NULLIF(?, ''), notes),
			updated_at          = CURRENT_TIMESTAMP
		WHERE id = ?`,
		string(p), evidence, id,
	); err != nil {
		return fmt.Errorf("set presence for %s: %w", id, err)
	}

	if p == PresenceAbsent {
		if _, err := tx.Exec(`
			UPDATE brokers SET status = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND status = ?`,
			string(StatusSkipped), id, string(StatusPending),
		); err != nil {
			return fmt.Errorf("skip absent broker %s: %w", id, err)
		}
	}

	return tx.Commit()
}

// PresenceStats counts brokers by presence finding across the whole registry.
func (s *Store) PresenceStats() (map[Presence]int, error) {
	rows, err := s.db.Query(`SELECT presence, COUNT(*) FROM brokers GROUP BY presence`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[Presence]int{}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			return nil, err
		}
		out[Presence(p)] = n
	}
	return out, rows.Err()
}

// CompletionStats counts successes by how they were actually completed.
// This is the honest automation-coverage number: autonomous successes are
// what the code achieved on its own, everything else is a human doing the
// work with the tool's help.
func (s *Store) CompletionStats() (map[CompletionMethod]int, error) {
	rows, err := s.db.Query(`
		SELECT completion_method, COUNT(*) FROM brokers
		WHERE status = 'success' GROUP BY completion_method`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[CompletionMethod]int)
	for rows.Next() {
		var m string
		var n int
		if err := rows.Scan(&m, &n); err != nil {
			return nil, err
		}
		out[CompletionMethod(m)] = n
	}
	return out, rows.Err()
}

// BlockerStats counts brokers by blocker_type, ignoring unclassified ones.
func (s *Store) BlockerStats() (map[BlockerType]int, error) {
	rows, err := s.db.Query(`
		SELECT blocker_type, COUNT(*) FROM brokers
		WHERE blocker_type != '' GROUP BY blocker_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[BlockerType]int)
	for rows.Next() {
		var b string
		var n int
		if err := rows.Scan(&b, &n); err != nil {
			return nil, err
		}
		out[BlockerType(b)] = n
	}
	return out, rows.Err()
}

// OrphanedBrokerIDs returns broker rows present in the database but absent
// from the canonical registry passed in (dashboard.AllBrokers()).
//
// Seed() is INSERT OR IGNORE, so removing or renaming a broker in code never
// deletes its row - `peeplookup` sat in the database for days after being
// replaced by `checkpeople`, silently inflating every count that read from
// the DB while the code said something different. The registry is intent;
// the database is live state; this reports where they have drifted apart
// instead of letting the two quietly disagree.
func (s *Store) OrphanedBrokerIDs(registry []Broker) ([]string, error) {
	known := make(map[string]bool, len(registry))
	for _, b := range registry {
		known[b.ID] = true
	}

	rows, err := s.db.Query(`SELECT id FROM brokers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orphans []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if !known[id] {
			orphans = append(orphans, id)
		}
	}
	return orphans, rows.Err()
}

// Stats returns aggregate counts by status.
func (s *Store) Stats() (map[Status]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM brokers GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[Status]int)
	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			return nil, err
		}
		out[Status(st)] = count
	}
	return out, rows.Err()
}

// Reset sets a broker back to pending (for re-testing). Use carefully.
func (s *Store) Reset(id string) error {
	_, err := s.db.Exec(`
		UPDATE brokers SET status='pending', attempt_count=0,
		last_attempt_at=NULL, confirmation_url=NULL, notes=NULL,
		updated_at=CURRENT_TIMESTAMP WHERE id=?`, id,
	)
	return err
}

// LastLiveAttemptAt returns when this broker was last dispatched for a real
// attempt that actually reached the site, or nil if never. Reads the attempts
// log, not the broker row - Reset wipes brokers.last_attempt_at for a clean
// display slate, but attempts rows are permanent, so this survives a Reset.
// That's what lets the orchestrator's cooldown guard catch "reset then
// immediately retried," which a check against brokers.last_attempt_at alone
// would miss entirely.
//
// StatusManual attempts are excluded deliberately. Manual routing means the
// agent declined to act - no verified handler, or a required input like
// profile_url missing - and returns before any navigation happens, so not a
// single request reaches the broker. The cooldown exists to keep the tool
// from looking like abusive repeat traffic to a site; an outcome that never
// contacted the site can't contribute to that and must not gate a later real
// attempt. (Found when a "no profile_url set" result blocked Spokeo's first
// genuine run for 48h despite never having touched spokeo.com.)
func (s *Store) LastLiveAttemptAt(id string) (*time.Time, error) {
	// MAX() on a TIMESTAMP column loses the type-affinity hint the driver
	// uses to auto-convert direct column selects into time.Time (compare
	// scanBrokers, which scans last_attempt_at straight into *time.Time
	// with no issue) - an aggregate result comes back as plain TEXT here,
	// so it has to be parsed manually. Found by actually exercising this
	// against a broker with real attempt history (go test never did -
	// every existing test broker had zero attempts, where MAX() returns
	// SQL NULL and the type mismatch never triggers).
	var raw sql.NullString
	err := s.db.QueryRow(`
		SELECT MAX(created_at) FROM attempts
		WHERE broker_id = ? AND dry_run = 0 AND status != 'manual'`, id,
	).Scan(&raw)
	if err != nil {
		return nil, err
	}
	if !raw.Valid {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", raw.String)
	if err != nil {
		return nil, fmt.Errorf("parse attempts.created_at %q: %w", raw.String, err)
	}
	return &t, nil
}

// AllowedUser represents a user with access to the admin panel.
type AllowedUser struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// SubjectProfile holds the subject's PII profile.
type SubjectProfile struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	State string `json:"state"`
}

// BootstrapAllowedUsers seeds the UNBROKERRDD_ADMIN_EMAIL if the table is empty.
func (s *Store) BootstrapAllowedUsers(adminEmail string) error {
	if adminEmail == "" {
		return nil
	}
	// Check if any rows exist in allowed_users
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM allowed_users").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = s.db.Exec("INSERT INTO allowed_users (email, role) VALUES (?, 'admin')", adminEmail)
		if err != nil {
			return fmt.Errorf("bootstrap admin user: %w", err)
		}
		log.Printf("[db] Bootstrapped admin user: %s", adminEmail)
	}
	return nil
}

// GetUserRole returns the role of the user, defaulting to "viewer" if they do not exist.
func (s *Store) GetUserRole(email string) (string, error) {
	var role string
	err := s.db.QueryRow("SELECT role FROM allowed_users WHERE email = ?", email).Scan(&role)
	if err == sql.ErrNoRows {
		return "viewer", nil
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

// ListAllowedUsers returns all allowed users.
func (s *Store) ListAllowedUsers() ([]AllowedUser, error) {
	rows, err := s.db.Query("SELECT email, role FROM allowed_users ORDER BY email")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AllowedUser
	for rows.Next() {
		var u AllowedUser
		if err := rows.Scan(&u.Email, &u.Role); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// AddAllowedUser adds a new allowed user.
func (s *Store) AddAllowedUser(email, role string) error {
	if role != "viewer" && role != "admin" {
		return fmt.Errorf("invalid role: %s", role)
	}
	_, err := s.db.Exec("INSERT INTO allowed_users (email, role) VALUES (?, ?)", email, role)
	return err
}

// UpdateAllowedUserRole updates an existing user's role. Enforces the last-admin invariant.
func (s *Store) UpdateAllowedUserRole(email, role string) error {
	if role != "viewer" && role != "admin" {
		return fmt.Errorf("invalid role: %s", role)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentRole string
	err = tx.QueryRow("SELECT role FROM allowed_users WHERE email = ?", email).Scan(&currentRole)
	if err != nil {
		return err
	}

	if currentRole == "admin" && role == "viewer" {
		var adminCount int
		err = tx.QueryRow("SELECT COUNT(*) FROM allowed_users WHERE role = 'admin'").Scan(&adminCount)
		if err != nil {
			return err
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot demote the last admin")
		}
	}

	_, err = tx.Exec("UPDATE allowed_users SET role = ? WHERE email = ?", role, email)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteAllowedUser removes a user. Enforces the last-admin invariant.
func (s *Store) DeleteAllowedUser(email string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentRole string
	err = tx.QueryRow("SELECT role FROM allowed_users WHERE email = ?", email).Scan(&currentRole)
	if err == sql.ErrNoRows {
		return fmt.Errorf("user not found")
	}
	if err != nil {
		return err
	}

	if currentRole == "admin" {
		var adminCount int
		err = tx.QueryRow("SELECT COUNT(*) FROM allowed_users WHERE role = 'admin'").Scan(&adminCount)
		if err != nil {
			return err
		}
		if adminCount <= 1 {
			return fmt.Errorf("cannot remove the last admin")
		}
	}

	_, err = tx.Exec("DELETE FROM allowed_users WHERE email = ?", email)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetSubjectProfile retrieves the single subject profile (id = 1).
func (s *Store) GetSubjectProfile() (*SubjectProfile, error) {
	var p SubjectProfile
	err := s.db.QueryRow("SELECT name, email, state FROM subject_profile WHERE id = 1").Scan(&p.Name, &p.Email, &p.State)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateSubjectProfile updates the single subject profile (id = 1).
func (s *Store) UpdateSubjectProfile(p *SubjectProfile) error {
	_, err := s.db.Exec(`
		INSERT INTO subject_profile (id, name, email, state)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			email = excluded.email,
			state = excluded.state
	`, p.Name, p.Email, p.State)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func scanBrokers(rows *sql.Rows) ([]Broker, error) {
	var out []Broker
	for rows.Next() {
		var b Broker
		var lat, pat *time.Time
		var blockerType, coveredBy, profileURL, completionMethod, presence string
		if err := rows.Scan(
			&b.ID, &b.Name, &b.Strategy, &b.URL, &b.Status,
			&b.AttemptCount, &lat, &b.ConfirmationURL, &b.Notes,
			&blockerType, &coveredBy, &profileURL, &completionMethod,
			&presence, &pat,
		); err != nil {
			return nil, err
		}
		b.LastAttemptAt = lat
		b.PresenceCheckedAt = pat
		b.Presence = Presence(presence)
		b.BlockerType = BlockerType(blockerType)
		b.CoveredBy = coveredBy
		b.ProfileURL = profileURL
		b.CompletionMethod = CompletionMethod(completionMethod)
		out = append(out, b)
	}
	return out, rows.Err()
}
