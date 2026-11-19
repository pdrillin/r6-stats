package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/store"
)

func testMatch() *match.Match {
	players := []rec.Player{
		{Name: "a1", Team: 0, Side: rec.Attack, Operator: "ASH"},
		{Name: "b1", Team: 1, Side: rec.Defense, Operator: "MUTE"},
	}
	h := rec.Header{
		MatchID:   "m1",
		Timestamp: "2026-08-15-13-39-14",
		Map:       "Chalet",
		Teams:     [2]rec.Team{{Name: "A"}, {Name: "B"}},
		Players:   players,
	}
	r := &rec.Round{
		Complete: true,
		Winner:   0,
		Kills: []rec.Kill{{Killer: "a1", KillerTeam: 0, Victim: "b1", VictimTeam: 1, Headshot: true,
			Clock: rec.Clock{Phase: rec.PhaseAction, ElapsedMs: 30000}}},
		Scoreboards: map[string]rec.Scoreboard{"a1": {Assists: 2}},
		WinReason:   rec.DefendersEliminated,
	}
	return &match.Match{Map: "Chalet", Teams: [2]string{"A", "B"},
		Rounds: []match.RoundFile{{Header: h, Round: r}}}
}

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

func TestMatches(t *testing.T) {
	s := New([]*match.Match{testMatch()}, Options{})

	var list []MatchSummary
	if code := get(t, s, "/api/matches", &list); code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	want := MatchSummary{ID: "m1", Map: "Chalet", Teams: [2]string{"A", "B"}, Score: [2]int{1, 0},
		Date: "2026-08-15T13:39:14", RoundCount: 1, Kind: store.Pracc}
	if list[0] != want {
		t.Errorf("summary = %+v, want %+v", list[0], want)
	}

	var d MatchDetail
	if code := get(t, s, "/api/matches/m1", &d); code != 200 {
		t.Fatalf("detail: %d", code)
	}
	r := d.Rounds[0]
	if r.Number != 1 || r.Attacker != 0 || r.Winner != 0 || len(r.Kills) != 1 {
		t.Errorf("round = %+v", r)
	}
	a1 := r.Players[0]
	if a1.Name != "a1" || a1.Kills != 1 || a1.Headshots != 1 || a1.Assists != 2 || a1.Operator != "ASH" {
		t.Errorf("a1 = %+v", a1)
	}
	if b1 := r.Players[1]; b1.Deaths != 1 || b1.Kills != 0 {
		t.Errorf("b1 = %+v", b1)
	}
	if d.Stats == nil || d.Stats.Rounds != 1 {
		t.Errorf("match stats = %+v", d.Stats)
	}

	var o Overview
	if code := get(t, s, "/api/overview", &o); code != 200 || o.Matches != 1 || o.Stats.Rounds != 1 {
		t.Errorf("overview: %d %+v", code, o)
	}

	if o.OperatorSides["ASH"] != rec.Attack || o.OperatorSides["MUTE"] != rec.Defense {
		t.Errorf("operator sides = %v", o.OperatorSides)
	}
	if o.Map != "" || len(o.Maps) != 1 || o.Maps[0] != "Chalet" {
		t.Errorf("overview maps: %q %v", o.Map, o.Maps)
	}

	for _, url := range []string{"/api/matches/nope", "/api/nope", "/api/overview?map=Nope"} {
		if code := get(t, s, url, nil); code != 404 {
			t.Errorf("%s: %d, want 404", url, code)
		}
	}
}

func TestOverviewByMap(t *testing.T) {
	bank := testMatch()
	bank.Map = "Bank"
	bank.Rounds[0].Header.MatchID = "m2"
	other := testMatch()
	other.Rounds[0].Header.MatchID = "m3"
	// The last one is the first match loaded twice: counted once.
	s := New([]*match.Match{testMatch(), bank, other, testMatch()}, Options{})

	for url, want := range map[string]struct {
		matches int
		mapName string
	}{
		"/api/overview":            {3, ""},
		"/api/overview?map=Chalet": {2, "Chalet"},
		"/api/overview?map=Bank":   {1, "Bank"},
	} {
		var o Overview
		if code := get(t, s, url, &o); code != 200 {
			t.Fatalf("%s: %d", url, code)
		}
		if o.Matches != want.matches || o.Map != want.mapName || o.Stats.Rounds != want.matches {
			t.Errorf("%s: map %q, %d matches, %d rounds; want %q, %d", url, o.Map, o.Matches, o.Stats.Rounds, want.mapName, want.matches)
		}
		if len(o.Maps) != 2 || o.Maps[0] != "Bank" || o.Maps[1] != "Chalet" {
			t.Errorf("%s: maps %v, want [Bank Chalet]", url, o.Maps)
		}
	}
}

// send makes a request with a JSON body and returns the status.
func send(t *testing.T, h http.Handler, method, url, contentType, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	io.Copy(io.Discard, rr.Body)
	return rr.Code
}

func TestKinds(t *testing.T) {
	bank := testMatch()
	bank.Map = "Bank"
	bank.Rounds[0].Header.MatchID = "m2"
	st, err := store.Open(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := New([]*match.Match{testMatch(), bank}, Options{Store: st})

	overview := func(url string) Overview {
		t.Helper()
		var o Overview
		if code := get(t, s, url, &o); code != 200 {
			t.Fatalf("%s: %d", url, code)
		}
		return o
	}
	// Unlabelled matches are praccs.
	if o := overview("/api/overview?kind=pracc"); o.Matches != 2 || o.Kind != store.Pracc {
		t.Errorf("praccs before labelling: %d", o.Matches)
	}
	if o := overview("/api/overview?kind=official"); o.Matches != 0 {
		t.Errorf("officials before labelling: %d", o.Matches)
	}

	if code := send(t, s, "PUT", "/api/matches/m2/kind", "application/json", `{"kind":"official"}`); code != 204 {
		t.Fatalf("label: %d", code)
	}
	// The cached overviews were dropped.
	if o := overview("/api/overview?kind=official"); o.Matches != 1 || o.Stats.Rounds != 1 {
		t.Errorf("officials after labelling: %d matches, %d rounds", o.Matches, o.Stats.Rounds)
	}
	if o := overview("/api/overview?kind=official&map=Chalet"); o.Matches != 0 {
		t.Errorf("official matches on Chalet: %d", o.Matches)
	}
	if o := overview("/api/overview"); o.Matches != 2 {
		t.Errorf("all matches: %d", o.Matches)
	}
	var d MatchDetail
	get(t, s, "/api/matches/m2", &d)
	var list []MatchSummary
	get(t, s, "/api/matches", &list)
	if d.Kind != store.Official || list[0].Kind != store.Pracc || list[1].Kind != store.Official {
		t.Errorf("kinds: detail %q, list %q %q", d.Kind, list[0].Kind, list[1].Kind)
	}
	if st.Kind("m2") != store.Official {
		t.Error("label not stored")
	}

	for _, c := range []struct {
		url, contentType, body string
		want                   int
	}{
		{"/api/matches/nope/kind", "application/json", `{"kind":"official"}`, 404},
		{"/api/matches/m2/kind", "application/json", `{"kind":"ranked"}`, 400},
		{"/api/matches/m2/kind", "application/json", `{"kind":"pracc","x":1}`, 400},
		{"/api/matches/m2/kind", "application/json", `not json`, 400},
		// Plain forms are refused: another site could post them.
		{"/api/matches/m2/kind", "application/x-www-form-urlencoded", `kind=pracc`, 415},
		{"/api/matches/m2/kind", "text/plain", `{"kind":"pracc"}`, 415},
	} {
		if code := send(t, s, "PUT", c.url, c.contentType, c.body); code != c.want {
			t.Errorf("PUT %s %s %s: %d, want %d", c.url, c.contentType, c.body, code, c.want)
		}
	}
	if st.Kind("m2") != store.Official {
		t.Error("a refused request changed the label")
	}
	if code := get(t, s, "/api/overview?kind=ranked", nil); code != 400 {
		t.Errorf("unknown kind filter: %d", code)
	}
}

func TestSettings(t *testing.T) {
	st := store.Memory()
	s := New([]*match.Match{testMatch()}, Options{Store: st})
	var got Settings
	if code := get(t, s, "/api/settings", &got); code != 200 || got.MyTeam != "" {
		t.Fatalf("settings: %d %+v", code, got)
	}
	if code := send(t, s, "PUT", "/api/settings", "application/json", `{"myTeam":"A"}`); code != 204 {
		t.Fatalf("put: %d", code)
	}
	get(t, s, "/api/settings", &got)
	if got.MyTeam != "A" || st.MyTeam() != "A" {
		t.Errorf("team = %q, stored %q", got.MyTeam, st.MyTeam())
	}
	if code := send(t, s, "PUT", "/api/settings", "text/plain", `{"myTeam":"B"}`); code != 415 {
		t.Errorf("text/plain: %d", code)
	}
}

func TestSPA(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "secret"), []byte("secret"), 0o644)
	dir := filepath.Join(root, "web")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("js"), 0o644)
	s := New(nil, Options{Web: dir})

	for url, want := range map[string]string{
		"/":              "index",
		"/matches/m1":    "index", // a route of the interface
		"/assets/app.js": "js",
	} {
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, httptest.NewRequest("GET", url, nil))
		if body := strings.TrimSpace(rr.Body.String()); body != want {
			t.Errorf("%s = %q, want %q", url, body, want)
		}
	}

	// The mux redirects paths with "..", but spa must stay inside dir on
	// its own.
	rr := httptest.NewRecorder()
	spa(dir).ServeHTTP(rr, httptest.NewRequest("GET", "/../secret", nil))
	if strings.Contains(rr.Body.String(), "secret") {
		t.Errorf("file outside dir served")
	}
}
