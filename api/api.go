// Package api serves match statistics as JSON for the web interface.
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/stats"
	"github.com/pdrillin/r6-stats/store"
)

// MatchSummary is one line of the match list.
type MatchSummary struct {
	ID    string    `json:"id"`
	Map   string    `json:"map"`
	Teams [2]string `json:"teams"`
	Score [2]int    `json:"score"`
	// Date is when the first round was recorded, as the game wrote it
	// (local time of the recording machine, no zone).
	Date       string `json:"date"`
	RoundCount int    `json:"roundCount"`
	// Superseded counts recordings of rounds replayed after a rehost.
	Superseded int `json:"superseded"`
	// Kind is what the match was played for, as the user labelled it.
	Kind store.Kind `json:"kind"`
}

// MatchDetail is a match with every round and its own statistics.
type MatchDetail struct {
	MatchSummary
	Rounds []Round       `json:"rounds"`
	Stats  *stats.Report `json:"stats"`
}

// Round is one round of a match as the interface shows it.
type Round struct {
	Number int `json:"number"` // from 1
	// Score is the score before the round.
	Score [2]int `json:"score"`
	// Attacker is the index of the attacking team, -1 when unknown.
	Attacker  int           `json:"attacker"`
	Site      string        `json:"site"`
	Winner    int           `json:"winner"`
	WinReason rec.WinReason `json:"winReason"`
	Plant     *rec.Clock    `json:"plant"`
	Planter   string        `json:"planter"`
	Defuser   string        `json:"defuser"`
	End       rec.Clock     `json:"end"`
	Kills     []rec.Kill    `json:"kills"`
	Players   []RoundPlayer `json:"players"`
}

// RoundPlayer is what one player did in a round.
type RoundPlayer struct {
	Name     string   `json:"name"`
	Team     int      `json:"team"`
	Side     rec.Side `json:"side"`
	Operator string   `json:"operator"` // empty when the header does not tell
	Kills    int      `json:"kills"`
	Deaths   int      `json:"deaths"`
	Assists  int      `json:"assists"`
	// Headshots counts the player's kills that were headshots.
	Headshots int `json:"headshots"`
}

// Overview is the statistics over every match, or over the matches of one
// map or one kind.
type Overview struct {
	// Map is the map the statistics are restricted to, empty for all.
	Map string `json:"map"`
	// Kind is the kind of match they are restricted to, empty for all.
	Kind store.Kind `json:"kind"`
	// Maps lists every map played, whatever the filter, sorted.
	Maps    []string      `json:"maps"`
	Matches int           `json:"matches"`
	Stats   *stats.Report `json:"stats"`
	// TradeWindowMs is the trade window the statistics were computed with.
	TradeWindowMs int `json:"tradeWindowMs"`
	// OperatorSides tells the side of every operator seen in the headers:
	// an operator only plays one side.
	OperatorSides map[string]rec.Side `json:"operatorSides"`
}

// Settings are the user's choices.
type Settings struct {
	MyTeam string `json:"myTeam"`
}

// Options configure a Server.
type Options struct {
	// Web is a folder holding the built interface, served for every path
	// outside /api/. Empty serves the API only.
	Web string
	// Store holds what the user tells about the matches. Nil keeps it in
	// memory.
	Store *store.Store
}

// Server answers the API requests. Match details are computed when the
// server is built; overviews when first asked for, and again once the user
// relabels a match.
type Server struct {
	matches []*match.Match // without duplicates, in order
	details map[string]*MatchDetail
	ids     []string // id of each match
	maps    []string
	sides   map[string]rec.Side
	store   *store.Store
	mux     *http.ServeMux

	mu        sync.Mutex
	overviews map[[2]string]*Overview // by map and kind
}

// New builds the server for the given matches.
func New(matches []*match.Match, opt Options) *Server {
	s := &Server{
		details:   map[string]*MatchDetail{},
		maps:      []string{},
		sides:     map[string]rec.Side{},
		store:     opt.Store,
		mux:       http.NewServeMux(),
		overviews: map[[2]string]*Overview{},
	}
	if s.store == nil {
		s.store = store.Memory()
	}
	seenMap := map[string]bool{}
	for _, m := range matches {
		d := detail(m)
		// Two matches can share the id of their first recording only if
		// the same file is loaded twice; keep the first.
		if _, dup := s.details[d.ID]; dup {
			continue
		}
		s.details[d.ID] = d
		s.matches = append(s.matches, m)
		s.ids = append(s.ids, d.ID)
		if !seenMap[d.Map] {
			seenMap[d.Map] = true
			s.maps = append(s.maps, d.Map)
		}
		for _, rf := range m.Rounds {
			for _, p := range rf.Header.Players {
				if p.Operator != "" && (p.Side == rec.Attack || p.Side == rec.Defense) {
					s.sides[p.Operator] = p.Side
				}
			}
		}
	}
	sort.Strings(s.maps)

	s.mux.HandleFunc("GET /api/overview", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mapName, kind := q.Get("map"), store.Kind(q.Get("kind"))
		if mapName != "" && !seenMap[mapName] {
			http.Error(w, "unknown map", http.StatusNotFound)
			return
		}
		if kind != "" && !kind.Valid() {
			http.Error(w, "unknown match kind", http.StatusBadRequest)
			return
		}
		writeJSON(w, s.overview(mapName, kind))
	})
	s.mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		list := make([]MatchSummary, len(s.ids))
		for i, id := range s.ids {
			list[i] = s.details[id].MatchSummary
			list[i].Kind = s.store.Kind(id)
		}
		writeJSON(w, list)
	})
	s.mux.HandleFunc("GET /api/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
		d := s.details[r.PathValue("id")]
		if d == nil {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		}
		out := *d
		out.Kind = s.store.Kind(d.ID)
		writeJSON(w, out)
	})
	s.mux.HandleFunc("PUT /api/matches/{id}/kind", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if s.details[id] == nil {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		}
		var body struct {
			Kind store.Kind `json:"kind"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		if !body.Kind.Valid() {
			http.Error(w, "unknown match kind", http.StatusBadRequest)
			return
		}
		if err := s.store.SetKind(id, body.Kind); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Statistics filtered by kind are now out of date.
		s.mu.Lock()
		clear(s.overviews)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Settings{MyTeam: s.store.MyTeam()})
	})
	s.mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var body Settings
		if !readJSON(w, r, &body) {
			return
		}
		if err := s.store.SetMyTeam(body.MyTeam); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	if opt.Web != "" {
		s.mux.Handle("/", spa(opt.Web))
	}
	return s
}

// overview returns the statistics over the matches of a map and a kind,
// empty meaning all.
func (s *Server) overview(mapName string, kind store.Kind) *Overview {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := [2]string{mapName, string(kind)}
	if o := s.overviews[key]; o != nil {
		return o
	}
	var ms []*match.Match
	for i, m := range s.matches {
		if (mapName == "" || mapName == mapNameOf(m)) && (kind == "" || kind == s.store.Kind(s.ids[i])) {
			ms = append(ms, m)
		}
	}
	o := &Overview{
		Map:           mapName,
		Kind:          kind,
		Maps:          s.maps,
		Matches:       len(ms),
		Stats:         stats.Compute(stats.Options{}, ms...),
		TradeWindowMs: stats.DefaultTradeWindowMs,
		OperatorSides: s.sides,
	}
	s.overviews[key] = o
	return o
}

// readJSON decodes a request body. Only JSON is accepted: browsers send JSON
// across origins only after a preflight this server never approves, so
// another website cannot write here through the visitor's browser.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if ct := r.Header.Get("Content-Type"); ct != "application/json" && !strings.HasPrefix(ct, "application/json;") {
		http.Error(w, "send application/json", http.StatusUnsupportedMediaType)
		return false
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// spa serves the files of dir, and index.html for any path that is not a
// file, so the interface's own routes load the application.
func spa(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}

// mapNameOf is the map's name, or its id when the map is unknown.
func mapNameOf(m *match.Match) string {
	if m.Map == "" {
		return "#" + m.MapID
	}
	return m.Map
}

func detail(m *match.Match) *MatchDetail {
	first := m.Rounds[0].Header
	name := mapNameOf(m)
	d := &MatchDetail{
		MatchSummary: MatchSummary{
			ID:         first.MatchID,
			Map:        name,
			Teams:      m.Teams,
			Score:      m.Score(),
			Date:       isoDate(first.Timestamp),
			RoundCount: len(m.Rounds),
			Superseded: len(m.Superseded),
		},
		Rounds: make([]Round, len(m.Rounds)),
		Stats:  stats.Compute(stats.Options{}, m),
	}
	if d.ID == "" {
		d.ID = strings.ReplaceAll(first.Timestamp, "-", "")
	}
	for i, rf := range m.Rounds {
		d.Rounds[i] = round(rf, m.Roster(i))
	}
	return d
}

func round(rf match.RoundFile, roster []rec.Player) Round {
	h, r := rf.Header, rf.Round
	out := Round{
		Number:    h.RoundNumber + 1,
		Score:     [2]int{h.Teams[0].StartingScore, h.Teams[1].StartingScore},
		Attacker:  -1,
		Site:      r.Site,
		Winner:    r.Winner,
		WinReason: r.WinReason,
		Plant:     r.Plant,
		Planter:   r.Planter,
		Defuser:   r.Defuser,
		End:       r.End,
		Kills:     r.Kills,
		Players:   []RoundPlayer{},
	}
	if out.Kills == nil {
		out.Kills = []rec.Kill{}
	}
	for _, p := range roster {
		if p.Team != 0 && p.Team != 1 {
			continue
		}
		if p.Side == rec.Attack {
			out.Attacker = p.Team
		}
		rp := RoundPlayer{Name: p.Name, Team: p.Team, Side: p.Side, Operator: p.Operator,
			Assists: r.Scoreboards[p.Name].Assists}
		for _, k := range r.Kills {
			// Team kills are not counted as kills, like in the statistics.
			if k.Killer == p.Name && k.KillerTeam != k.VictimTeam {
				rp.Kills++
				if k.Headshot {
					rp.Headshots++
				}
			}
			if k.Victim == p.Name {
				rp.Deaths++
			}
		}
		out.Players = append(out.Players, rp)
	}
	return out
}

// isoDate turns the game's timestamp (2006-01-02-15-04-05) into ISO 8601
// without a zone. Unknown formats are returned as is.
func isoDate(ts string) string {
	t, err := time.Parse("2006-01-02-15-04-05", ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02T15:04:05")
}
