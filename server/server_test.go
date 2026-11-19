package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pdrillin/r6-stats/api"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/store"
)

const corpusDir = "../Replays"

// chalet returns the replays of the shortest match of the corpus (Chalet,
// 7-0), or skips the test without the corpus.
func chalet(t *testing.T) []Upload {
	t.Helper()
	var paths []string
	filepath.WalkDir(corpusDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(p, "Chalet") && strings.EqualFold(filepath.Ext(p), ".rec") {
			paths = append(paths, p)
		}
		return nil
	})
	if len(paths) == 0 {
		t.Skipf("no Chalet match under %s", corpusDir)
	}
	sort.Strings(paths)
	var out []Upload
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Upload{Name: filepath.Base(p), Data: data})
	}
	return out
}

func open(t *testing.T, dir string) *Server {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func space(t *testing.T, s *Server, name string) string {
	t.Helper()
	sp, err := s.CreateSpace(name, Team)
	if err != nil {
		t.Fatal(err)
	}
	return sp.ID
}

func add(t *testing.T, s *Server, id string, files []Upload, want Status) {
	t.Helper()
	for _, u := range files {
		res, err := s.Add(id, u)
		if err != nil {
			t.Fatal(err)
		}
		if res.Status != want {
			t.Fatalf("%s: %s (%s), want %s", u.Name, res.Status, res.Error, want)
		}
	}
}

func count(t *testing.T, s *Server, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// get calls the API of the server and decodes the answer.
func get(t *testing.T, h http.Handler, url string, v any) int {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", url, nil))
	if v != nil && rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
			t.Fatalf("%s: %v", url, err)
		}
	}
	return rr.Code
}

func TestSpaces(t *testing.T) {
	s := open(t, t.TempDir())
	for _, c := range []struct {
		name string
		kind SpaceKind
	}{{"", Team}, {"   ", Team}, {strings.Repeat("x", 81), Team}, {"A", "club"}} {
		if _, err := s.CreateSpace(c.name, c.kind); err == nil {
			t.Errorf("space %q %q accepted", c.name, c.kind)
		}
	}
	a := space(t, s, "Ma team")
	b, err := s.CreateSpace("Ma ligue", League)
	if err != nil {
		t.Fatal(err)
	}
	if a == b.ID || len(a) != 16 {
		t.Errorf("ids %q %q", a, b.ID)
	}
	list, err := s.Spaces()
	if err != nil || len(list) != 2 || list[0].Name != "Ma team" || list[1].Kind != League {
		t.Errorf("spaces = %+v, %v", list, err)
	}

	if _, err := s.Add("nope", Upload{Name: "x.rec", Data: []byte("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown space: %v", err)
	}
	res, err := s.Add(a, Upload{Name: "notes.txt", Data: []byte("not a replay")})
	if err != nil || res.Status != Invalid {
		t.Errorf("invalid file: %+v, %v", res, err)
	}
	big := Upload{Name: "big.rec", Data: make([]byte, MaxReplaySize+1)}
	if res, _ := s.Add(a, big); res.Status != Invalid {
		t.Errorf("oversized file: %+v", res)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM replay_files"); n != 0 {
		t.Errorf("%d files stored for invalid uploads", n)
	}
}

func TestAddAndView(t *testing.T) {
	files := chalet(t)
	dir := t.TempDir()
	s := open(t, dir)
	a := space(t, s, "A")
	b := space(t, s, "B")

	add(t, s, a, files, Added)
	add(t, s, a, files, Already)
	// The other space gets its own links, without storing or decoding
	// anything again.
	add(t, s, b, files, Added)
	if n := count(t, s, "SELECT COUNT(*) FROM replay_files"); n != len(files) {
		t.Errorf("%d raw files stored, want %d", n, len(files))
	}
	if n := count(t, s, "SELECT COUNT(*) FROM rounds WHERE parser_version = ?", rec.Version); n != len(files) {
		t.Errorf("%d decoded rounds, want %d", n, len(files))
	}
	if n := count(t, s, "SELECT COUNT(*) FROM space_files WHERE space_id = ?", b); n != len(files) {
		t.Errorf("space B holds %d files", n)
	}

	h := s.Handler()
	var list []api.MatchSummary
	if code := get(t, h, "/api/spaces/"+a+"/matches", &list); code != 200 || len(list) != 1 {
		t.Fatalf("matches: %d %+v", code, list)
	}
	m := list[0]
	if m.Map != "Chalet" || m.Score != [2]int{7, 0} || m.RoundCount != 7 || m.Kind != store.Pracc {
		t.Errorf("match = %+v", m)
	}

	// Labels and settings are per space and survive a restart.
	put := func(url, body string) {
		t.Helper()
		req := httptest.NewRequest("PUT", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("PUT %s: %d %s", url, rr.Code, rr.Body)
		}
	}
	put("/api/spaces/"+a+"/matches/"+m.ID+"/kind", `{"kind":"official"}`)
	put("/api/spaces/"+a+"/settings", `{"myTeam":"DZ"}`)
	s.Close()
	s = open(t, dir)
	h = s.Handler()
	var o api.Overview
	get(t, h, "/api/spaces/"+a+"/overview?kind=official", &o)
	if o.Matches != 1 || o.Stats.Rounds != 7 {
		t.Errorf("officials of A after restart: %d matches, %d rounds", o.Matches, o.Stats.Rounds)
	}
	get(t, h, "/api/spaces/"+b+"/overview?kind=official", &o)
	if o.Matches != 0 {
		t.Errorf("A's label leaked into B: %d officials", o.Matches)
	}
	var st api.Settings
	get(t, h, "/api/spaces/"+a+"/settings", &st)
	if st.MyTeam != "DZ" {
		t.Errorf("my team after restart = %q", st.MyTeam)
	}
	get(t, h, "/api/spaces/"+b+"/settings", &st)
	if st.MyTeam != "" {
		t.Errorf("B's team = %q", st.MyTeam)
	}
}

// TestWithoutRaw checks the promise of the storage: once decoded, a match is
// shown from the database alone.
func TestWithoutRaw(t *testing.T) {
	files := chalet(t)
	dir := t.TempDir()
	s := open(t, dir)
	a := space(t, s, "A")
	add(t, s, a, files, Added)

	if err := os.RemoveAll(filepath.Join(dir, "raw")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE replay_files SET raw_path = NULL"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = open(t, dir)
	var o api.Overview
	if code := get(t, s.Handler(), "/api/spaces/"+a+"/overview", &o); code != 200 || o.Matches != 1 || o.Stats.Rounds != 7 {
		t.Errorf("without raw files: %d, %d matches, %d rounds", code, o.Matches, o.Stats.Rounds)
	}

	// The whole header is kept, not only the fields the code uses.
	var block []byte
	s.db.QueryRow("SELECT data FROM rounds LIMIT 1").Scan(&block)
	d, err := readBlock(block)
	if err != nil || len(d.Properties) < 50 || d.Header.Map != "Chalet" {
		t.Errorf("stored block: %d properties, map %q, %v", len(d.Properties), d.Header.Map, err)
	}

	// Rounds of an older parser cannot be decoded again without their raw
	// file: they keep their decoding.
	s.db.Exec("UPDATE rounds SET parser_version = 0")
	if n, err := s.Redecode(); err != nil || n != 0 {
		t.Errorf("redecode without raw files: %d, %v", n, err)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM rounds"); n != len(files) {
		t.Errorf("%d rounds left", n)
	}
}

func TestRedecode(t *testing.T) {
	files := chalet(t)
	s := open(t, t.TempDir())
	a := space(t, s, "A")
	add(t, s, a, files, Added)
	s.db.Exec("UPDATE rounds SET parser_version = 0, data = x'00'")
	n, err := s.Redecode()
	if err != nil || n != len(files) {
		t.Fatalf("redecode: %d, %v", n, err)
	}
	var o api.Overview
	if get(t, s.Handler(), "/api/spaces/"+a+"/overview", &o); o.Stats.Rounds != 7 {
		t.Errorf("after redecode: %d rounds", o.Stats.Rounds)
	}
	// Sending a file decoded by an older parser decodes it again too.
	s.db.Exec("UPDATE rounds SET parser_version = 0")
	add(t, s, a, files[:1], Already)
	if n := count(t, s, "SELECT COUNT(*) FROM rounds WHERE parser_version = ?", rec.Version); n != 1 {
		t.Errorf("%d rounds at the current version after sending one", n)
	}
}

// multipartBody builds an upload request body.
func multipartBody(t *testing.T, files []Upload) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("files", f.Name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(f.Data)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestHTTP(t *testing.T) {
	files := chalet(t)
	s := open(t, t.TempDir())
	h := s.Handler()

	send := func(method, url, contentType string, body io.Reader, header bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, url, body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if header {
			req.Header.Set("X-R6-Upload", "1")
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	rr := send("POST", "/api/spaces", "application/json", strings.NewReader(`{"name":"Team","kind":"team"}`), false)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body)
	}
	var sp Space
	json.Unmarshal(rr.Body.Bytes(), &sp)
	if rr := send("POST", "/api/spaces", "text/plain", strings.NewReader(`{"name":"x","kind":"team"}`), false); rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("create as text: %d", rr.Code)
	}

	upload := append([]Upload{{Name: "readme.txt", Data: []byte("hello")}}, files...)
	body, ct := multipartBody(t, upload)
	raw := body.Bytes()
	if rr := send("POST", "/api/spaces/"+sp.ID+"/replays", ct, bytes.NewReader(raw), false); rr.Code != http.StatusForbidden {
		t.Errorf("upload without header: %d", rr.Code)
	}
	if rr := send("POST", "/api/spaces/nope/replays", ct, bytes.NewReader(raw), true); rr.Code != http.StatusNotFound {
		t.Errorf("upload to unknown space: %d", rr.Code)
	}
	rr = send("POST", "/api/spaces/"+sp.ID+"/replays", ct, bytes.NewReader(raw), true)
	if rr.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rr.Code, rr.Body)
	}
	var results []Result
	json.Unmarshal(rr.Body.Bytes(), &results)
	if len(results) != len(upload) || results[0].Status != Invalid || results[0].Name != "readme.txt" {
		t.Fatalf("results = %+v", results)
	}
	for i, r := range results[1:] {
		if r.Status != Added || r.Name != files[i].Name {
			t.Errorf("result %d = %+v", i+1, r)
		}
	}

	var spaces []Space
	get(t, h, "/api/spaces", &spaces)
	if len(spaces) != 1 || spaces[0].Files != len(files) {
		t.Errorf("spaces = %+v", spaces)
	}
	var list []api.MatchSummary
	if get(t, h, "/api/spaces/"+sp.ID+"/matches", &list); len(list) != 1 {
		t.Errorf("%d matches after upload", len(list))
	}
	for _, url := range []string{"/api/spaces/nope/overview", "/api/spaces/" + sp.ID + "/nope", "/api/nope"} {
		if code := get(t, h, url, nil); code != http.StatusNotFound {
			t.Errorf("%s: %d", url, code)
		}
	}
}

func TestNewerSchemaRefused(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	s.db.Exec("PRAGMA user_version = 99")
	s.Close()
	if _, err := Open(dir); err == nil {
		t.Error("database from a newer server opened")
	}
}
