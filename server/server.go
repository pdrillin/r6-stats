package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"

	"github.com/pdrillin/r6-stats/api"
	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/store"
)

// Server holds the spaces and their replays.
type Server struct {
	db  *sql.DB
	dir string
	cfg Config // set by Handler

	mu    sync.Mutex
	views map[string]*api.Server // per space, built on demand
}

// Open opens the server's data in dir: the database and the raw replays.
func Open(dir string) (*Server, error) {
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return nil, err
	}
	db, err := openDB(dir)
	if err != nil {
		return nil, err
	}
	return &Server{db: db, dir: dir, views: map[string]*api.Server{}}, nil
}

// Close closes the database.
func (s *Server) Close() error { return s.db.Close() }

// Ping checks that the database answers, for health checks.
func (s *Server) Ping() error {
	var n int
	return s.db.QueryRow("SELECT 1").Scan(&n)
}

// Backup writes a consistent copy of the database to path, while the server
// keeps running. Raw replays are plain files: copy them as they are.
func (s *Server) Backup(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	// VACUUM INTO writes a snapshot of one transaction: never a half-written
	// state, unlike copying the file.
	_, err := s.db.Exec("VACUUM INTO ?", path)
	return err
}

// ErrNotFound is returned for a space that does not exist.
var ErrNotFound = errors.New("not found")

// SpaceKind tells what a space is for.
type SpaceKind string

const (
	Team   SpaceKind = "team"
	League SpaceKind = "league"
)

// Space is a set of matches shared by its members.
type Space struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      SpaceKind `json:"kind"`
	CreatedAt string    `json:"createdAt"`
	Files     int       `json:"files"` // replays held
	// Role is the role of the user who asked, in the lists of a user's
	// spaces.
	Role Role `json:"role,omitempty"`
}

// CreateSpace adds a space. A non-empty owner becomes its manager; a space
// created from the command line has none until a manager invitation is
// accepted.
func (s *Server) CreateSpace(name string, kind SpaceKind, owner string) (Space, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Space{}, fmt.Errorf("a space needs a name of 1 to 80 characters")
	}
	if kind != Team && kind != League {
		return Space{}, fmt.Errorf("unknown space kind %q", kind)
	}
	sp := Space{ID: newID(), Name: name, Kind: kind, CreatedAt: now()}
	tx, err := s.db.Begin()
	if err != nil {
		return Space{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO spaces (id, name, kind, created_at) VALUES (?, ?, ?, ?)",
		sp.ID, sp.Name, sp.Kind, sp.CreatedAt); err != nil {
		return Space{}, err
	}
	if owner != "" {
		sp.Role = Manager
		if _, err := tx.Exec("INSERT INTO memberships (space_id, user_id, role, joined_at) VALUES (?, ?, 'manager', ?)",
			sp.ID, owner, now()); err != nil {
			return Space{}, err
		}
	}
	return sp, tx.Commit()
}

// Spaces lists every space, oldest first.
func (s *Server) Spaces() ([]Space, error) {
	return s.querySpaces(`
		SELECT s.id, s.name, s.kind, s.created_at, (SELECT COUNT(*) FROM space_files f WHERE f.space_id = s.id), ''
		FROM spaces s ORDER BY s.rowid`)
}

// SpacesOf lists the spaces a user is a member of, with their role.
func (s *Server) SpacesOf(userID string) ([]Space, error) {
	return s.querySpaces(`
		SELECT s.id, s.name, s.kind, s.created_at, (SELECT COUNT(*) FROM space_files f WHERE f.space_id = s.id), m.role
		FROM spaces s JOIN memberships m ON m.space_id = s.id AND m.user_id = ?
		ORDER BY s.rowid`, userID)
}

func (s *Server) querySpaces(query string, args ...any) ([]Space, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Space{}
	for rows.Next() {
		var sp Space
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Kind, &sp.CreatedAt, &sp.Files, &sp.Role); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Server) spaceExists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM spaces WHERE id = ?", id).Scan(&n)
	return n > 0, err
}

// newID returns a random identifier, hard to guess: 16 characters of
// base32, 80 bits.
func newID() string {
	b := make([]byte, 10)
	rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b))
}

// ---------------------------------------------------------------------------
// Replays

// MaxReplaySize is the largest replay accepted. Round files of the corpus
// weigh 0.5 to 10 MB.
const MaxReplaySize = 64 << 20

// Upload is a replay sent to a space.
type Upload struct {
	Name string // file name as sent, e.g. Match-…-R01.rec
	Data []byte
	By   string // id of the user who sent it, empty from the command line
}

// Status says what became of an upload.
type Status string

const (
	Added   Status = "added"   // new in the space
	Already Status = "already" // the space already held it
	Invalid Status = "invalid" // not a replay, or one that cannot be decoded
	Ignored Status = "ignored" // a replay of a game spaces do not keep (ranked, arcade…)
)

// Result is the outcome of one upload.
type Result struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256,omitempty"`
	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`
}

// roundData is what is kept of a decoded replay: everything the interface
// needs, so that the raw file can be deleted.
type roundData struct {
	Header     rec.Header     `json:"header"`
	Properties []rec.Property `json:"properties"` // every header property as stored
	Round      *rec.Round     `json:"round"`
}

var (
	zenc, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	zdec, _ = zstd.NewReader(nil)
)

// Add stores a replay in a space: the raw file and its decoded round are
// kept once whatever the number of spaces holding it, and only decoded again
// when they were decoded by an older parser.
func (s *Server) Add(spaceID string, u Upload) (Result, error) {
	res := Result{Name: filepath.Base(u.Name)}
	if ok, err := s.spaceExists(spaceID); err != nil {
		return res, err
	} else if !ok {
		return res, ErrNotFound
	}
	if len(u.Data) > MaxReplaySize {
		res.Status, res.Error = Invalid, "file too large"
		return res, nil
	}
	sum := sha256.Sum256(u.Data)
	res.SHA256 = hex.EncodeToString(sum[:])

	// Matchmade games are played with strangers: they would blur a team's
	// statistics and expose people who never joined the space, so they are
	// not stored at all.
	f, err := rec.Parse(u.Data)
	if err != nil {
		res.Status, res.Error = Invalid, err.Error()
		return res, nil
	}
	if !f.Header.CustomGame() {
		res.Status, res.Error = Ignored, "not a custom game"
		return res, nil
	}

	// Decode first: nothing is stored for a file that is not a replay.
	version, err := s.roundVersion(res.SHA256)
	if err != nil {
		return res, err
	}
	var block []byte
	if version != rec.Version {
		block, err = decode(u.Data)
		if err != nil {
			res.Status, res.Error = Invalid, err.Error()
			return res, nil
		}
	}

	if err := s.storeRaw(res.SHA256, u.Data); err != nil {
		return res, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO replay_files (sha256, size, received_at, raw_path) VALUES (?, ?, ?, ?)
		ON CONFLICT (sha256) DO UPDATE SET raw_path = excluded.raw_path`,
		res.SHA256, len(u.Data), now(), rawPath(res.SHA256)); err != nil {
		return res, err
	}
	if block != nil {
		if _, err := tx.Exec(`INSERT INTO rounds (sha256, parser_version, data, decoded_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (sha256) DO UPDATE SET parser_version = excluded.parser_version,
				data = excluded.data, decoded_at = excluded.decoded_at`,
			res.SHA256, rec.Version, block, now()); err != nil {
			return res, err
		}
	}
	var by any
	if u.By != "" {
		by = u.By
	}
	r, err := tx.Exec(`INSERT INTO space_files (space_id, sha256, file_name, added_at, uploaded_by) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, spaceID, res.SHA256, res.Name, now(), by)
	if err != nil {
		return res, err
	}
	if err := tx.Commit(); err != nil {
		return res, err
	}
	res.Status = Already
	if n, _ := r.RowsAffected(); n > 0 {
		res.Status = Added
	}
	if res.Status == Added || block != nil {
		s.forget(spaceID)
	}
	return res, nil
}

// roundVersion is the parser version of the stored round of a replay, 0 when
// there is none.
func (s *Server) roundVersion(sha string) (int, error) {
	var v int
	err := s.db.QueryRow("SELECT parser_version FROM rounds WHERE sha256 = ?", sha).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// decode decodes a replay into the block kept in the database.
func decode(data []byte) ([]byte, error) {
	f, err := rec.Parse(data)
	if err != nil {
		return nil, err
	}
	r, err := f.Round()
	if err != nil {
		return nil, err
	}
	h := f.Header
	props := h.Properties
	h.Properties = nil
	b, err := json.Marshal(roundData{Header: h, Properties: props, Round: r})
	if err != nil {
		return nil, err
	}
	return zenc.EncodeAll(b, nil), nil
}

func readBlock(block []byte) (roundData, error) {
	b, err := zdec.DecodeAll(block, nil)
	if err != nil {
		return roundData{}, err
	}
	var d roundData
	if err := json.Unmarshal(b, &d); err != nil {
		return roundData{}, err
	}
	if d.Round == nil {
		return roundData{}, fmt.Errorf("no round in block")
	}
	return d, nil
}

// rawPath is where a raw replay is kept, relative to the raw folder: two
// levels of folders keep each one small.
func rawPath(sha string) string {
	return filepath.Join(sha[:2], sha[2:4], sha+".rec")
}

// storeRaw writes a raw replay unless it is already there.
func (s *Server) storeRaw(sha string, data []byte) error {
	path := filepath.Join(s.dir, "raw", rawPath(sha))
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Redecode decodes again, from their raw file, the rounds decoded by an
// older parser. Rounds whose raw file was deleted keep their old decoding.
// It returns how many rounds were decoded again.
func (s *Server) Redecode() (int, error) {
	rows, err := s.db.Query(`SELECT r.sha256, f.raw_path FROM rounds r JOIN replay_files f USING (sha256)
		WHERE r.parser_version <> ? AND f.raw_path IS NOT NULL`, rec.Version)
	if err != nil {
		return 0, err
	}
	type stale struct{ sha, path string }
	var todo []stale
	for rows.Next() {
		var st stale
		if err := rows.Scan(&st.sha, &st.path); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, st)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	done := 0
	for _, st := range todo {
		data, err := os.ReadFile(filepath.Join(s.dir, "raw", st.path))
		if err != nil {
			return done, err
		}
		block, err := decode(data)
		if err != nil {
			// The new parser fails where the old one succeeded: keep the
			// old decoding rather than lose the round.
			continue
		}
		if _, err := s.db.Exec("UPDATE rounds SET parser_version = ?, data = ?, decoded_at = ? WHERE sha256 = ?",
			rec.Version, block, now(), st.sha); err != nil {
			return done, err
		}
		done++
	}
	if done > 0 {
		s.mu.Lock()
		clear(s.views)
		s.mu.Unlock()
	}
	return done, nil
}

// ---------------------------------------------------------------------------
// Views

// view returns the API of a space, built from its rounds when needed.
func (s *Server) view(spaceID string) (*api.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v := s.views[spaceID]; v != nil {
		return v, nil
	}
	if ok, err := s.spaceExists(spaceID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrNotFound
	}
	matches, err := s.matches(spaceID)
	if err != nil {
		return nil, err
	}
	v := api.New(matches, api.Options{Store: spaceStore{s.db, spaceID}})
	s.views[spaceID] = v
	return v, nil
}

// forget drops the view of a space after its replays changed.
func (s *Server) forget(spaceID string) {
	s.mu.Lock()
	delete(s.views, spaceID)
	s.mu.Unlock()
}

// matches assembles the matches of a space from its decoded rounds.
func (s *Server) matches(spaceID string) ([]*match.Match, error) {
	ms, _, err := s.matchFiles(spaceID)
	return ms, err
}

// matchFiles assembles the matches of a space, and tells the replay (SHA-256)
// behind each decoded round.
func (s *Server) matchFiles(spaceID string) ([]*match.Match, map[*rec.Round]string, error) {
	shas := map[*rec.Round]string{}
	rows, err := s.db.Query(`SELECT f.file_name, f.sha256, r.data FROM space_files f JOIN rounds r USING (sha256)
		WHERE f.space_id = ?`, spaceID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var files []match.RoundFile
	for rows.Next() {
		var name, sha string
		var block []byte
		if err := rows.Scan(&name, &sha, &block); err != nil {
			return nil, nil, err
		}
		d, err := readBlock(block)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		shas[d.Round] = sha
		files = append(files, match.RoundFile{Path: name, Header: d.Header, Round: d.Round})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return match.Assemble(files), shas, nil
}

// spaceStore keeps the kinds and the own team of one space in the database.
type spaceStore struct {
	db *sql.DB
	id string
}

func (st spaceStore) Kind(matchID string) store.Kind {
	var k store.Kind
	err := st.db.QueryRow("SELECT kind FROM match_kinds WHERE space_id = ? AND match_id = ?", st.id, matchID).Scan(&k)
	if err != nil || !k.Valid() {
		return store.DefaultKind
	}
	return k
}

func (st spaceStore) SetKind(matchID string, k store.Kind) error {
	if !k.Valid() {
		return fmt.Errorf("unknown match kind %q", k)
	}
	_, err := st.db.Exec(`INSERT INTO match_kinds (space_id, match_id, kind) VALUES (?, ?, ?)
		ON CONFLICT (space_id, match_id) DO UPDATE SET kind = excluded.kind`, st.id, matchID, k)
	return err
}

func (st spaceStore) MyTeam() string {
	var t string
	st.db.QueryRow("SELECT my_team FROM spaces WHERE id = ?", st.id).Scan(&t)
	return t
}

func (st spaceStore) SetMyTeam(team string) error {
	_, err := st.db.Exec("UPDATE spaces SET my_team = ? WHERE id = ?", team, st.id)
	return err
}
