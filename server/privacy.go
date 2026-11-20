package server

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdrillin/r6-stats/api"
	"github.com/pdrillin/r6-stats/match"
)

// What a user can delete, and what they can take with them (GDPR rights of
// erasure and access). A replay is kept while at least one space holds it:
// once the last link goes, its decoded rounds and raw file go too.

// ErrSoleManager refuses to delete an account that is the only manager of a
// space where others remain: someone must manage it first.
type ErrSoleManager struct{ Spaces []string }

func (e ErrSoleManager) Error() string {
	return "name another manager first in: " + strings.Join(e.Spaces, ", ")
}

// DeleteMatch removes a match from a space: the space's links to all its
// recordings, superseded ones included, and its label.
func (s *Server) DeleteMatch(spaceID, matchID string) error {
	ms, shas, err := s.matchFiles(spaceID)
	if err != nil {
		return err
	}
	var files []string
	for _, m := range ms {
		if api.MatchID(m) != matchID {
			continue
		}
		for _, rf := range append(append([]match.RoundFile{}, m.Rounds...), m.Superseded...) {
			files = append(files, shas[rf.Round])
		}
	}
	if len(files) == 0 {
		return ErrNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, sha := range files {
		if _, err := tx.Exec("DELETE FROM space_files WHERE space_id = ? AND sha256 = ?", spaceID, sha); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("DELETE FROM match_kinds WHERE space_id = ? AND match_id = ?", spaceID, matchID); err != nil {
		return err
	}
	raw, err := collect(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.removeRaw(raw)
	s.forget(spaceID)
	return nil
}

// ClearSpace removes every match of a space and their labels, and returns
// how many replays it held. Members, invitations, the own team and the
// team names stay: they apply again to replays sent afterwards.
func (s *Server) ClearSpace(spaceID string) (int, error) {
	if ok, err := s.spaceExists(spaceID); err != nil {
		return 0, err
	} else if !ok {
		return 0, ErrNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	r, err := tx.Exec("DELETE FROM space_files WHERE space_id = ?", spaceID)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	if _, err := tx.Exec("DELETE FROM match_kinds WHERE space_id = ?", spaceID); err != nil {
		return 0, err
	}
	raw, err := collect(tx)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.removeRaw(raw)
	s.forget(spaceID)
	return int(n), nil
}

// DeleteSpace removes a space with everything only it held.
func (s *Server) DeleteSpace(spaceID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.Exec("DELETE FROM spaces WHERE id = ?", spaceID)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	raw, err := collect(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.removeRaw(raw)
	s.forget(spaceID)
	return nil
}

// DeleteAccount removes a user: sessions and memberships go, the replays
// they sent stay in their spaces without their name. Spaces where they were
// the only member go with them; spaces where they are the only manager of
// other members must be handed over first.
func (s *Server) DeleteAccount(userID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT s.id, s.name,
			(SELECT COUNT(*) FROM memberships o WHERE o.space_id = s.id AND o.user_id <> ?),
			(SELECT COUNT(*) FROM memberships o WHERE o.space_id = s.id AND o.user_id <> ? AND o.role = 'manager')
		FROM spaces s JOIN memberships m ON m.space_id = s.id AND m.user_id = ?`, userID, userID, userID)
	if err != nil {
		return err
	}
	var alone []string
	var blocked []string
	for rows.Next() {
		var id, name string
		var others, otherManagers int
		if err := rows.Scan(&id, &name, &others, &otherManagers); err != nil {
			rows.Close()
			return err
		}
		switch {
		case others == 0:
			alone = append(alone, id)
		case otherManagers == 0:
			var role Role
			if err := tx.QueryRow("SELECT role FROM memberships WHERE space_id = ? AND user_id = ?", id, userID).Scan(&role); err != nil {
				rows.Close()
				return err
			}
			if role == Manager {
				blocked = append(blocked, name)
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(blocked) > 0 {
		return ErrSoleManager{blocked}
	}
	for _, id := range alone {
		if _, err := tx.Exec("DELETE FROM spaces WHERE id = ?", id); err != nil {
			return err
		}
	}
	r, err := tx.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	raw, err := collect(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.removeRaw(raw)
	s.mu.Lock()
	clear(s.views)
	s.mu.Unlock()
	return nil
}

// collect deletes the replays no space holds any more, and returns the raw
// files to remove once the transaction is committed.
func collect(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT sha256, COALESCE(raw_path, '') FROM replay_files
		WHERE sha256 NOT IN (SELECT sha256 FROM space_files)`)
	if err != nil {
		return nil, err
	}
	var shas, raw []string
	for rows.Next() {
		var sha, path string
		if err := rows.Scan(&sha, &path); err != nil {
			rows.Close()
			return nil, err
		}
		shas = append(shas, sha)
		if path != "" {
			raw = append(raw, path)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, sha := range shas {
		if _, err := tx.Exec("DELETE FROM rounds WHERE sha256 = ?", sha); err != nil {
			return nil, err
		}
		if _, err := tx.Exec("DELETE FROM replay_files WHERE sha256 = ?", sha); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

// removeRaw deletes raw replay files, and the folders they leave empty. A
// file that cannot be removed only costs disk space: it is logged.
func (s *Server) removeRaw(paths []string) {
	for _, p := range paths {
		path := filepath.Join(s.dir, "raw", p)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "removing raw replay %s: %v\n", p, err)
			continue
		}
		// The two folder levels go too once empty; Remove refuses a
		// folder that still holds files.
		dir := filepath.Dir(path)
		if os.Remove(dir) == nil {
			os.Remove(filepath.Dir(dir))
		}
	}
}

// Export is everything kept about a user, for the right of access.
type Export struct {
	User    User           `json:"user"`
	Created string         `json:"createdAt"`
	Spaces  []Space        `json:"spaces"`
	Uploads []ExportUpload `json:"uploads"`
	Note    string         `json:"note"`
}

// ExportUpload is a replay the user sent.
type ExportUpload struct {
	Space    string `json:"space"`
	FileName string `json:"fileName"`
	SHA256   string `json:"sha256"`
	AddedAt  string `json:"addedAt"`
}

// ExportUser gathers the data kept about a user.
func (s *Server) ExportUser(userID string) (Export, error) {
	var e Export
	err := s.db.QueryRow("SELECT id, discord_id, name, avatar, created_at FROM users WHERE id = ?", userID).
		Scan(&e.User.ID, &e.User.DiscordID, &e.User.Name, &e.User.Avatar, &e.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	if err != nil {
		return e, err
	}
	if e.Spaces, err = s.SpacesOf(userID); err != nil {
		return e, err
	}
	rows, err := s.db.Query(`SELECT sp.name, f.file_name, f.sha256, f.added_at FROM space_files f
		JOIN spaces sp ON sp.id = f.space_id WHERE f.uploaded_by = ? ORDER BY f.added_at`, userID)
	if err != nil {
		return e, err
	}
	defer rows.Close()
	e.Uploads = []ExportUpload{}
	for rows.Next() {
		var u ExportUpload
		if err := rows.Scan(&u.Space, &u.FileName, &u.SHA256, &u.AddedAt); err != nil {
			return e, err
		}
		e.Uploads = append(e.Uploads, u)
	}
	e.Note = "Les statistiques de jeu de chaque espace se consultent dans l'espace lui-même ; " +
		"les replays contiennent aussi les pseudos des autres joueurs du match."
	return e, rows.Err()
}
