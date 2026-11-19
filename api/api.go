// Package api serves match statistics as JSON for the web interface.
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
	"github.com/pdrillin/r6-stats/stats"
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
// map.
type Overview struct {
	// Map is the map the statistics are restricted to, empty for all.
	Map string `json:"map"`
	// Maps lists every map played, whatever the filter, sorted.
	Maps    []string      `json:"maps"`
	Matches int           `json:"matches"`
	Stats   *stats.Report `json:"stats"`
	// TradeWindowMs is the trade window the statistics were computed with.
	TradeWindowMs int `json:"tradeWindowMs"`
}

// Server answers the API requests. Everything is computed once, when the
// server is built.
type Server struct {
	overviews map[string]*Overview // by map name, "" for all maps
	list      []MatchSummary
	details   map[string]*MatchDetail
	mux       *http.ServeMux
}

// New builds the server for the given matches. When web is not empty it is a
// folder holding the built interface, served for every path outside /api/.
func New(matches []*match.Match, web string) *Server {
	s := &Server{
		overviews: map[string]*Overview{},
		list:      []MatchSummary{},
		details:   map[string]*MatchDetail{},
		mux:       http.NewServeMux(),
	}
	byMap := map[string][]*match.Match{"": matches}
	maps := []string{}
	for _, m := range matches {
		name := mapName(m)
		if byMap[name] == nil {
			maps = append(maps, name)
		}
		byMap[name] = append(byMap[name], m)
	}
	sort.Strings(maps)
	for name, ms := range byMap {
		s.overviews[name] = &Overview{
			Map:           name,
			Maps:          maps,
			Matches:       len(ms),
			Stats:         stats.Compute(stats.Options{}, ms...),
			TradeWindowMs: stats.DefaultTradeWindowMs,
		}
	}
	for _, m := range matches {
		d := detail(m)
		// Two matches can share the id of their first recording only if
		// the same file is loaded twice; keep the first.
		if _, dup := s.details[d.ID]; dup {
			continue
		}
		s.details[d.ID] = d
		s.list = append(s.list, d.MatchSummary)
	}

	s.mux.HandleFunc("GET /api/overview", func(w http.ResponseWriter, r *http.Request) {
		o := s.overviews[r.URL.Query().Get("map")]
		if o == nil {
			http.Error(w, "unknown map", http.StatusNotFound)
			return
		}
		writeJSON(w, o)
	})
	s.mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.list)
	})
	s.mux.HandleFunc("GET /api/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
		d := s.details[r.PathValue("id")]
		if d == nil {
			http.Error(w, "match not found", http.StatusNotFound)
			return
		}
		writeJSON(w, d)
	})
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	if web != "" {
		s.mux.Handle("/", spa(web))
	}
	return s
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

// mapName is the map's name, or its id when the map is unknown.
func mapName(m *match.Match) string {
	if m.Map == "" {
		return "#" + m.MapID
	}
	return m.Map
}

func detail(m *match.Match) *MatchDetail {
	first := m.Rounds[0].Header
	name := mapName(m)
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
