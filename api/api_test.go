package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
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
	s := New([]*match.Match{testMatch()}, "")

	var list []MatchSummary
	if code := get(t, s, "/api/matches", &list); code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	want := MatchSummary{ID: "m1", Map: "Chalet", Teams: [2]string{"A", "B"}, Score: [2]int{1, 0},
		Date: "2026-08-15T13:39:14", RoundCount: 1}
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
	s := New([]*match.Match{testMatch(), bank, testMatch()}, "")

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

func TestSPA(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "secret"), []byte("secret"), 0o644)
	dir := filepath.Join(root, "web")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("js"), 0o644)
	s := New(nil, dir)

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
