// Package db manages broker state in a local SQLite database.
// The database is the source of truth — the dashboard reads from it,
// and agents write to it. It never leaves the local machine.
package db

import (
	"database/sql"
	"fmt"
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

// Broker is one row in the brokers table.
type Broker struct {
	ID              string
	Name            string
	Strategy        int
	URL             string
	Status          Status
	AttemptCount    int
	LastAttemptAt   *time.Time
	ConfirmationURL string
	Notes           string
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
	`)
	return err
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
func (s *Store) Settle(id string, status Status, dryRun bool, result, notes, confirmURL string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE brokers SET
			status           = ?,
			attempt_count    = attempt_count + 1,
			last_attempt_at  = CURRENT_TIMESTAMP,
			confirmation_url = COALESCE(NULLIF(?, ''), confirmation_url),
			notes            = COALESCE(NULLIF(?, ''), notes),
			updated_at       = CURRENT_TIMESTAMP
		WHERE id = ?`,
		string(status), confirmURL, notes, id,
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
		       last_attempt_at, COALESCE(confirmation_url,''), COALESCE(notes,'')
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
		       last_attempt_at, COALESCE(confirmation_url,''), COALESCE(notes,'')
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

func (s *Store) Close() error { return s.db.Close() }

func scanBrokers(rows *sql.Rows) ([]Broker, error) {
	var out []Broker
	for rows.Next() {
		var b Broker
		var lat *time.Time
		if err := rows.Scan(
			&b.ID, &b.Name, &b.Strategy, &b.URL, &b.Status,
			&b.AttemptCount, &lat, &b.ConfirmationURL, &b.Notes,
		); err != nil {
			return nil, err
		}
		b.LastAttemptAt = lat
		out = append(out, b)
	}
	return out, rows.Err()
}
