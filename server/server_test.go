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
	"net/url"
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

const testURL = "http://r6.test"

// client calls the server as one signed-in user (or nobody, with an empty
// session).
type client struct {
	t       *testing.T
	h       http.Handler
	user    User
	session string
}

func newClient(t *testing.T, s *Server, cfg Config, name string) *client {
	t.Helper()
	c := &client{t: t, h: s.Handler(cfg)}
	if name != "" {
		u, secret, err := s.SignIn("discord-"+name, name, "")
		if err != nil {
			t.Fatal(err)
		}
		c.user, c.session = u, secret
	}
	return c
}

func (c *client) do(method, url, contentType string, body io.Reader, header map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	req := httptest.NewRequest(method, url, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if c.session != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.session})
	}
	rr := httptest.NewRecorder()
	c.h.ServeHTTP(rr, req)
	return rr
}

// get decodes the answer into v when the status is 200, and returns the status.
func (c *client) get(url string, v any) int {
	c.t.Helper()
	rr := c.do("GET", url, "", nil, nil)
	if v != nil && rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
			c.t.Fatalf("%s: %v", url, err)
		}
	}
	return rr.Code
}

// send makes a JSON request, decodes a JSON answer into v, returns the status.
func (c *client) send(method, url, body string, v any) int {
	c.t.Helper()
	rr := c.do(method, url, "application/json", strings.NewReader(body), nil)
	if v != nil && rr.Code < 300 {
		if err := json.Unmarshal(rr.Body.Bytes(), v); err != nil {
			c.t.Fatalf("%s %s: %v (%s)", method, url, err, rr.Body)
		}
	}
	return rr.Code
}

func (c *client) createSpace(name string) Space {
	c.t.Helper()
	var sp Space
	if code := c.send("POST", "/api/spaces", `{"name":"`+name+`","kind":"team"}`, &sp); code != http.StatusCreated {
		c.t.Fatalf("create space: %d", code)
	}
	return sp
}

// upload sends replays through the HTTP API.
func (c *client) upload(spaceID string, files []Upload) (int, []Result) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		w, err := mw.CreateFormFile("files", f.Name)
		if err != nil {
			c.t.Fatal(err)
		}
		w.Write(f.Data)
	}
	mw.Close()
	rr := c.do("POST", "/api/spaces/"+spaceID+"/replays", mw.FormDataContentType(), &buf, map[string]string{"X-R6-Upload": "1"})
	var results []Result
	if rr.Code == http.StatusOK {
		json.Unmarshal(rr.Body.Bytes(), &results)
	}
	return rr.Code, results
}

func TestSpaces(t *testing.T) {
	s := open(t, t.TempDir())
	for _, c := range []struct {
		name string
		kind SpaceKind
	}{{"", Team}, {"   ", Team}, {strings.Repeat("x", 81), Team}, {"A", "club"}} {
		if _, err := s.CreateSpace(c.name, c.kind, ""); err == nil {
			t.Errorf("space %q %q accepted", c.name, c.kind)
		}
	}
	a, err := s.CreateSpace("Ma team", Team, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateSpace("Ma ligue", League, "")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || len(a.ID) != 16 {
		t.Errorf("ids %q %q", a.ID, b.ID)
	}
	list, err := s.Spaces()
	if err != nil || len(list) != 2 || list[0].Name != "Ma team" || list[1].Kind != League {
		t.Errorf("spaces = %+v, %v", list, err)
	}

	if _, err := s.Add("nope", Upload{Name: "x.rec", Data: []byte("x")}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown space: %v", err)
	}
	res, err := s.Add(a.ID, Upload{Name: "notes.txt", Data: []byte("not a replay")})
	if err != nil || res.Status != Invalid {
		t.Errorf("invalid file: %+v, %v", res, err)
	}
	big := Upload{Name: "big.rec", Data: make([]byte, MaxReplaySize+1)}
	if res, _ := s.Add(a.ID, big); res.Status != Invalid {
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
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	a := alice.createSpace("A").ID
	b := alice.createSpace("B").ID

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

	var list []api.MatchSummary
	if code := alice.get("/api/spaces/"+a+"/matches", &list); code != 200 || len(list) != 1 {
		t.Fatalf("matches: %d %+v", code, list)
	}
	m := list[0]
	if m.Map != "Chalet" || m.Score != [2]int{7, 0} || m.RoundCount != 7 || m.Kind != store.Pracc {
		t.Errorf("match = %+v", m)
	}

	// Labels and settings are per space and survive a restart.
	if code := alice.send("PUT", "/api/spaces/"+a+"/matches/"+m.ID+"/kind", `{"kind":"official"}`, nil); code != 204 {
		t.Fatalf("label: %d", code)
	}
	if code := alice.send("PUT", "/api/spaces/"+a+"/settings", `{"myTeam":"DZ"}`, nil); code != 204 {
		t.Fatalf("settings: %d", code)
	}
	s.Close()
	s = open(t, dir)
	alice.h = s.Handler(Config{BaseURL: testURL})
	var o api.Overview
	alice.get("/api/spaces/"+a+"/overview?kind=official", &o)
	if o.Matches != 1 || o.Stats.Rounds != 7 {
		t.Errorf("officials of A after restart: %d matches, %d rounds", o.Matches, o.Stats.Rounds)
	}
	alice.get("/api/spaces/"+b+"/overview?kind=official", &o)
	if o.Matches != 0 {
		t.Errorf("A's label leaked into B: %d officials", o.Matches)
	}
	var st api.Settings
	alice.get("/api/spaces/"+a+"/settings", &st)
	if st.MyTeam != "DZ" {
		t.Errorf("my team after restart = %q", st.MyTeam)
	}
}

// TestWithoutRaw checks the promise of the storage: once decoded, a match is
// shown from the database alone.
func TestWithoutRaw(t *testing.T) {
	files := chalet(t)
	dir := t.TempDir()
	s := open(t, dir)
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	a := alice.createSpace("A").ID
	add(t, s, a, files, Added)

	if err := os.RemoveAll(filepath.Join(dir, "raw")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE replay_files SET raw_path = NULL"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = open(t, dir)
	alice.h = s.Handler(Config{BaseURL: testURL})
	var o api.Overview
	if code := alice.get("/api/spaces/"+a+"/overview", &o); code != 200 || o.Matches != 1 || o.Stats.Rounds != 7 {
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
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	a := alice.createSpace("A").ID
	add(t, s, a, files, Added)
	s.db.Exec("UPDATE rounds SET parser_version = 0, data = x'00'")
	n, err := s.Redecode()
	if err != nil || n != len(files) {
		t.Fatalf("redecode: %d, %v", n, err)
	}
	var o api.Overview
	if alice.get("/api/spaces/"+a+"/overview", &o); o.Stats.Rounds != 7 {
		t.Errorf("after redecode: %d rounds", o.Stats.Rounds)
	}
	// Sending a file decoded by an older parser decodes it again too.
	s.db.Exec("UPDATE rounds SET parser_version = 0")
	add(t, s, a, files[:1], Already)
	if n := count(t, s, "SELECT COUNT(*) FROM rounds WHERE parser_version = ?", rec.Version); n != 1 {
		t.Errorf("%d rounds at the current version after sending one", n)
	}
}

func TestUpload(t *testing.T) {
	files := chalet(t)
	s := open(t, t.TempDir())
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	sp := alice.createSpace("Team")

	upload := append([]Upload{{Name: "readme.txt", Data: []byte("hello")}}, files...)
	code, results := alice.upload(sp.ID, upload)
	if code != http.StatusOK {
		t.Fatalf("upload: %d", code)
	}
	if len(results) != len(upload) || results[0].Status != Invalid || results[0].Name != "readme.txt" {
		t.Fatalf("results = %+v", results)
	}
	for i, r := range results[1:] {
		if r.Status != Added || r.Name != files[i].Name {
			t.Errorf("result %d = %+v", i+1, r)
		}
	}
	if n := count(t, s, "SELECT COUNT(*) FROM space_files WHERE uploaded_by = ?", alice.user.ID); n != len(files) {
		t.Errorf("%d files credited to the uploader", n)
	}

	// A plain form from another page lacks the header.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.CreateFormFile("files", "x.rec")
	mw.Close()
	if rr := alice.do("POST", "/api/spaces/"+sp.ID+"/replays", mw.FormDataContentType(), &buf, nil); rr.Code != http.StatusForbidden {
		t.Errorf("upload without header: %d", rr.Code)
	}

	var spaces []Space
	alice.get("/api/spaces", &spaces)
	if len(spaces) != 1 || spaces[0].Files != len(files) || spaces[0].Role != Manager {
		t.Errorf("spaces = %+v", spaces)
	}
	var list []api.MatchSummary
	if alice.get("/api/spaces/"+sp.ID+"/matches", &list); len(list) != 1 {
		t.Errorf("%d matches after upload", len(list))
	}
}

// TestAccess walks through who may do what.
func TestAccess(t *testing.T) {
	s := open(t, t.TempDir())
	cfg := Config{BaseURL: testURL, Creators: []string{"discord-alice"}}
	alice := newClient(t, s, cfg, "alice")
	bob := newClient(t, s, cfg, "bob")
	nobody := newClient(t, s, cfg, "")
	sp := alice.createSpace("Team")
	base := "/api/spaces/" + sp.ID

	// Signed out: nothing.
	for _, url := range []string{"/api/me", "/api/spaces", base + "/overview"} {
		if code := nobody.get(url, nil); code != http.StatusUnauthorized {
			t.Errorf("signed out %s: %d", url, code)
		}
	}
	// Bob is not in the creators list.
	if code := bob.send("POST", "/api/spaces", `{"name":"B","kind":"team"}`, nil); code != http.StatusForbidden {
		t.Errorf("bob creates a space: %d", code)
	}
	var me struct {
		User
		CanCreate bool `json:"canCreate"`
	}
	bob.get("/api/me", &me)
	if me.Name != "bob" || me.CanCreate {
		t.Errorf("bob's /api/me = %+v", me)
	}

	// Not a member: the space does not exist for him.
	for _, url := range []string{base + "/overview", base + "/matches", base + "/members"} {
		if code := bob.get(url, nil); code != http.StatusNotFound {
			t.Errorf("non-member %s: %d", url, code)
		}
	}
	if code, _ := bob.upload(sp.ID, nil); code != http.StatusNotFound {
		t.Errorf("non-member upload: %d", code)
	}
	var spaces []Space
	if bob.get("/api/spaces", &spaces); len(spaces) != 0 {
		t.Errorf("bob sees %d spaces", len(spaces))
	}

	// Only managers invite.
	var inv struct {
		Invitation
		URL string `json:"url"`
	}
	if code := alice.send("POST", base+"/invitations", `{"role":"player"}`, &inv); code != http.StatusCreated {
		t.Fatalf("invite: %d", code)
	}
	if !strings.HasPrefix(inv.URL, testURL+"/invite/") || inv.Token == "" {
		t.Errorf("invitation = %+v", inv)
	}
	var info InvitationInfo
	if code := bob.get("/api/invitations/"+inv.Token, &info); code != 200 || info.SpaceName != "Team" || info.Role != Player {
		t.Errorf("invitation info: %d %+v", code, info)
	}
	if code := bob.send("POST", "/api/invitations/"+inv.Token+"/accept", ``, nil); code != 200 {
		t.Fatalf("accept: %d", code)
	}

	// A player reads and uploads, but changes nothing else.
	if code := bob.get(base+"/overview", nil); code != 200 {
		t.Errorf("player reads: %d", code)
	}
	if code, _ := bob.upload(sp.ID, nil); code != 200 {
		t.Errorf("player uploads: %d", code)
	}
	for _, c := range []struct{ method, url, body string }{
		{"PUT", base + "/settings", `{"myTeam":"X"}`},
		{"PUT", base + "/matches/m/kind", `{"kind":"official"}`},
		{"POST", base + "/invitations", `{"role":"player"}`},
		{"PUT", base + "/members/" + alice.user.ID, `{"role":"player"}`},
		{"DELETE", base + "/members/" + alice.user.ID, ``},
	} {
		if code := bob.send(c.method, c.url, c.body, nil); code != http.StatusForbidden {
			t.Errorf("player %s %s: %d", c.method, c.url, code)
		}
	}
	if code := bob.get(base+"/invitations", nil); code != http.StatusForbidden {
		t.Errorf("player lists invitations: %d", code)
	}
	var members []Member
	bob.get(base+"/members", &members)
	if len(members) != 2 || members[0].Name != "alice" || members[0].Role != Manager || members[1].Role != Player {
		t.Errorf("members = %+v", members)
	}

	// A space keeps a manager.
	if code := alice.send("PUT", base+"/members/"+alice.user.ID, `{"role":"player"}`, nil); code != http.StatusConflict {
		t.Errorf("last manager steps down: %d", code)
	}
	if code := alice.send("DELETE", base+"/members/"+alice.user.ID, ``, nil); code != http.StatusConflict {
		t.Errorf("last manager leaves: %d", code)
	}
	if code := alice.send("PUT", base+"/members/"+bob.user.ID, `{"role":"captain"}`, nil); code != http.StatusBadRequest {
		t.Errorf("unknown role: %d", code)
	}
	if code := alice.send("PUT", base+"/members/"+bob.user.ID, `{"role":"manager"}`, nil); code != 204 {
		t.Errorf("promote bob: %d", code)
	}
	if code := alice.send("PUT", base+"/members/"+alice.user.ID, `{"role":"player"}`, nil); code != 204 {
		t.Errorf("alice steps down once bob manages: %d", code)
	}
	// Anyone may leave.
	if code := alice.send("DELETE", base+"/members/"+alice.user.ID, ``, nil); code != 204 {
		t.Errorf("alice leaves: %d", code)
	}
	if code := alice.get(base+"/overview", nil); code != http.StatusNotFound {
		t.Errorf("after leaving: %d", code)
	}

	// Invitations expire and can be revoked.
	var list []Invitation
	bob.get(base+"/invitations", &list)
	if len(list) != 1 || list[0].Token != "" || list[0].CreatedBy != "alice" {
		t.Errorf("invitations = %+v", list)
	}
	s.db.Exec("UPDATE invitations SET expires_at = '2000-01-01T00:00:00Z'")
	if code := alice.send("POST", "/api/invitations/"+inv.Token+"/accept", ``, nil); code != http.StatusGone {
		t.Errorf("expired invitation: %d", code)
	}
	var inv2 struct{ Invitation }
	bob.send("POST", base+"/invitations", `{"role":"manager"}`, &inv2)
	if code := bob.send("DELETE", base+"/invitations/"+inv2.ID, ``, nil); code != 204 {
		t.Errorf("revoke: %d", code)
	}
	if code := alice.get("/api/invitations/"+inv2.Token, nil); code != http.StatusGone {
		t.Errorf("revoked invitation: %d", code)
	}

	// Changes asked by another site are refused.
	rr := bob.do("PUT", base+"/settings", "application/json", strings.NewReader(`{"myTeam":"X"}`),
		map[string]string{"Origin": "https://evil.example"})
	if rr.Code != http.StatusForbidden {
		t.Errorf("cross-origin change: %d", rr.Code)
	}
	rr = bob.do("POST", "/auth/logout", "", nil, map[string]string{"Sec-Fetch-Site": "cross-site"})
	if rr.Code != http.StatusForbidden {
		t.Errorf("cross-site logout: %d", rr.Code)
	}

	// Signing out ends the session.
	if rr := bob.do("POST", "/auth/logout", "", nil, nil); rr.Code != http.StatusNoContent {
		t.Errorf("logout: %d", rr.Code)
	}
	if code := bob.get("/api/me", nil); code != http.StatusUnauthorized {
		t.Errorf("after logout: %d", code)
	}
}

// TestDiscordSignIn runs the OAuth2 flow against a fake Discord.
func TestDiscordSignIn(t *testing.T) {
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			id, secret, _ := r.BasicAuth()
			r.ParseForm()
			if id != "client" || secret != "secret" || r.Form.Get("code") != "good-code" ||
				r.Form.Get("redirect_uri") != testURL+"/auth/callback" {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			io.WriteString(w, `{"access_token":"tok","token_type":"Bearer"}`)
		case "/me":
			if r.Header.Get("Authorization") != "Bearer tok" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			io.WriteString(w, `{"id":"123","username":"pierre_r6","global_name":"Pierre","avatar":"abc"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer discord.Close()

	s := open(t, t.TempDir())
	cfg := Config{BaseURL: testURL, Discord: Discord{
		ClientID: "client", ClientSecret: "secret",
		AuthorizeURL: discord.URL + "/authorize", TokenURL: discord.URL + "/token", UserURL: discord.URL + "/me",
	}}
	c := newClient(t, s, cfg, "")

	// /auth/login sends the browser to Discord with a state kept in a
	// cookie; a next pointing elsewhere is dropped.
	rr := c.do("GET", "/auth/login?next="+url.QueryEscape("//evil.example/x"), "", nil, nil)
	if rr.Code != http.StatusFound {
		t.Fatalf("login: %d", rr.Code)
	}
	loc, _ := url.Parse(rr.Header().Get("Location"))
	q := loc.Query()
	if !strings.HasPrefix(loc.String(), discord.URL+"/authorize") || q.Get("client_id") != "client" ||
		q.Get("scope") != "identify" || q.Get("redirect_uri") != testURL+"/auth/callback" || q.Get("state") == "" {
		t.Fatalf("redirect = %s", loc)
	}
	var stateCookieValue string
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == stateCookie {
			stateCookieValue = ck.Value
		}
	}

	callback := func(state, code, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/auth/callback?state="+state+"&code="+code, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: stateCookie, Value: cookie})
		}
		rr := httptest.NewRecorder()
		c.h.ServeHTTP(rr, req)
		return rr
	}
	if rr := callback(q.Get("state"), "good-code", ""); rr.Code != http.StatusBadRequest {
		t.Errorf("callback without state cookie: %d", rr.Code)
	}
	if rr := callback("forged", "good-code", stateCookieValue); rr.Code != http.StatusBadRequest {
		t.Errorf("callback with another state: %d", rr.Code)
	}
	if rr := callback(q.Get("state"), "bad-code", stateCookieValue); rr.Code != http.StatusBadGateway {
		t.Errorf("callback with a refused code: %d", rr.Code)
	}
	rr = callback(q.Get("state"), "good-code", stateCookieValue)
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/" {
		t.Fatalf("callback: %d to %q", rr.Code, rr.Header().Get("Location"))
	}
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == sessionCookie {
			c.session = ck.Value
			if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
				t.Errorf("session cookie flags: %+v", ck)
			}
		}
	}
	var me User
	if code := c.get("/api/me", &me); code != 200 || me.Name != "Pierre" || me.DiscordID != "123" || me.Avatar != "abc" {
		t.Errorf("me = %d %+v", code, me)
	}
	// Signing in again is the same user.
	if _, _, err := s.SignIn("123", "Pierre R6", ""); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, "SELECT COUNT(*) FROM users"); n != 1 {
		t.Errorf("%d users after two sign-ins", n)
	}
	// The database keeps hashes, never the session secret itself.
	if n := count(t, s, "SELECT COUNT(*) FROM sessions WHERE token_hash = ?", c.session); n != 0 {
		t.Error("session secret stored in clear")
	}
}

func TestDevLogin(t *testing.T) {
	s := open(t, t.TempDir())
	off := newClient(t, s, Config{BaseURL: testURL}, "")
	if rr := off.do("GET", "/auth/dev?name=x", "", nil, nil); rr.Code != http.StatusNotFound {
		t.Errorf("dev login while off: %d", rr.Code)
	}
	on := newClient(t, s, Config{BaseURL: testURL, DevLogin: true}, "")
	rr := on.do("GET", "/auth/dev?name=coach&next=/x", "", nil, nil)
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != "/x" {
		t.Fatalf("dev login: %d %q", rr.Code, rr.Header().Get("Location"))
	}
	for _, ck := range rr.Result().Cookies() {
		if ck.Name == sessionCookie {
			on.session = ck.Value
		}
	}
	var me User
	if on.get("/api/me", &me); me.Name != "coach" {
		t.Errorf("dev user = %+v", me)
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

func TestHome(t *testing.T) {
	s := open(t, t.TempDir())
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	alice.createSpace("Ma team")
	home := s.Home()
	page := func(session, path string) (int, string) {
		req := httptest.NewRequest("GET", path, nil)
		if session != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
		}
		rr := httptest.NewRecorder()
		home.ServeHTTP(rr, req)
		return rr.Code, rr.Body.String()
	}
	if code, body := page("", "/"); code != 200 || !strings.Contains(body, `href="/auth/login"`) {
		t.Errorf("signed out: %d, no sign-in link", code)
	}
	if code, body := page(alice.session, "/"); code != 200 || !strings.Contains(body, "alice") || !strings.Contains(body, "Ma team") {
		t.Errorf("signed in: %d, name or space missing", code)
	}
	if code, _ := page(alice.session, "/elsewhere"); code != http.StatusNotFound {
		t.Errorf("other path: %d", code)
	}
	// Names come from Discord: they are escaped.
	s.db.Exec("UPDATE users SET name = '<script>x</script>'")
	if _, body := page(alice.session, "/"); strings.Contains(body, "<script>x") {
		t.Error("user name not escaped")
	}
}

func TestBackup(t *testing.T) {
	s := open(t, t.TempDir())
	alice := newClient(t, s, Config{BaseURL: testURL}, "alice")
	alice.createSpace("Ma team")
	if err := s.Ping(); err != nil {
		t.Fatal(err)
	}

	// A backup is a database the server opens as it is: restoring is
	// putting it back as r6stats.db.
	restored := t.TempDir()
	path := filepath.Join(restored, "r6stats.db")
	if err := s.Backup(path); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(path); err == nil {
		t.Error("backup overwrote an existing file")
	}
	r := open(t, restored)
	spaces, err := r.SpacesOf(alice.user.ID)
	if err != nil || len(spaces) != 1 || spaces[0].Name != "Ma team" || spaces[0].Role != Manager {
		t.Errorf("restored spaces = %+v, %v", spaces, err)
	}
	if u, ok := r.SessionUser(alice.session); !ok || u.Name != "alice" {
		t.Error("restored database lost the session")
	}
}
