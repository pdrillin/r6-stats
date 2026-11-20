// Package server is the shared server: spaces (a team, a league) whose
// members send replays, which are decoded once and kept as extracted data.
// Everything shown comes from that data; raw replays are only kept to decode
// them again when the parser improves.
//
// Storage is one SQLite database (tables for what is listed and filtered, one
// compressed JSON block per decoded round) and a folder of raw replays named
// by their SHA-256. Statistics are never stored: they are computed in memory
// from the rounds of a space.
package server

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// migrations bring the schema from one version to the next. Version n is
// reached by running migrations[n-1]. Never edit a migration that has
// shipped: add one.
var migrations = []string{
	`
CREATE TABLE spaces (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL,
	kind       TEXT NOT NULL CHECK (kind IN ('team', 'league')),
	-- plan will carry quotas (history, storage, raw retention).
	plan       TEXT NOT NULL DEFAULT 'beta',
	my_team    TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);

-- A replay received at least once, whichever space it was sent to.
CREATE TABLE replay_files (
	sha256      TEXT PRIMARY KEY,
	size        INTEGER NOT NULL,
	received_at TEXT NOT NULL,
	-- Path in the raw store; NULL once the raw file was deleted.
	raw_path    TEXT
);

-- The decoded round of a replay: zstd-compressed JSON (see roundData).
CREATE TABLE rounds (
	sha256         TEXT PRIMARY KEY REFERENCES replay_files (sha256),
	parser_version INTEGER NOT NULL,
	data           BLOB NOT NULL,
	decoded_at     TEXT NOT NULL
);

-- Which replays a space holds. A replay sent to two spaces is stored and
-- decoded once; each space only sees its own links.
CREATE TABLE space_files (
	space_id  TEXT NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
	sha256    TEXT NOT NULL REFERENCES replay_files (sha256),
	file_name TEXT NOT NULL,
	added_at  TEXT NOT NULL,
	PRIMARY KEY (space_id, sha256)
);

-- Kinds the members gave to matches; unlabelled matches use the default.
CREATE TABLE match_kinds (
	space_id TEXT NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
	match_id TEXT NOT NULL,
	kind     TEXT NOT NULL CHECK (kind IN ('pracc', 'official')),
	PRIMARY KEY (space_id, match_id)
);
`,
	`
-- A person, known through their Discord account.
CREATE TABLE users (
	id         TEXT PRIMARY KEY,
	discord_id TEXT NOT NULL UNIQUE,
	name       TEXT NOT NULL,
	avatar     TEXT NOT NULL DEFAULT '', -- Discord avatar hash
	created_at TEXT NOT NULL
);

-- A signed-in browser. Only the SHA-256 of the cookie is kept, so a leaked
-- database does not leak sessions.
CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);

CREATE TABLE memberships (
	space_id  TEXT NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
	user_id   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	role      TEXT NOT NULL CHECK (role IN ('manager', 'player')),
	joined_at TEXT NOT NULL,
	PRIMARY KEY (space_id, user_id)
);

-- A link to join a space, usable by several people until it expires or is
-- revoked. Only the SHA-256 of the token is kept.
CREATE TABLE invitations (
	id         TEXT PRIMARY KEY,
	token_hash TEXT NOT NULL UNIQUE,
	space_id   TEXT NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
	role       TEXT NOT NULL CHECK (role IN ('manager', 'player')),
	created_by TEXT REFERENCES users (id) ON DELETE SET NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);

ALTER TABLE space_files ADD COLUMN uploaded_by TEXT REFERENCES users (id) ON DELETE SET NULL;
`,
	`
-- Names the managers gave to teams, with their players (JSON list of
-- match.Alias): replays recorded by a player call every opponent "ENEMY TEAM".
ALTER TABLE spaces ADD COLUMN team_names TEXT NOT NULL DEFAULT '[]';
`,
}

// openDB opens the database in dir, creating it and bringing its schema up
// to date.
func openDB(dir string) (*sql.DB, error) {
	dsn := "file:" + filepath.Join(dir, "r6stats.db") +
		"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite has a single writer anyway, and this rules
	// out "database is locked" between our own goroutines.
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this server (%d)", version, len(migrations))
	}
	for v := version; v < len(migrations); v++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// now is the time written in the database: UTC, RFC 3339.
func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
