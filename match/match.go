// Package match assembles decoded round files into matches.
//
// The game writes one folder per hosted session. A ranked match is a single
// session, but a private match that is rehosted is split across several, and
// the round that was interrupted is recorded once per attempt.
package match

import (
	"sort"

	"github.com/pdrillin/stratdata/rec"
)

// RoundFile is one decoded .rec file.
type RoundFile struct {
	Path   string
	Header rec.Header
	Round  *rec.Round
}

// Match is one map played between two teams.
type Match struct {
	MapID string
	Map   string
	Teams [2]string
	// Rounds are the rounds that count, in order.
	Rounds []RoundFile
	// Superseded are recordings of a round that was played again after a
	// rehost.
	Superseded []RoundFile
}

// Score is the final score, or the score so far if the last round has no
// known winner.
func (m *Match) Score() [2]int {
	if len(m.Rounds) == 0 {
		return [2]int{}
	}
	last := m.Rounds[len(m.Rounds)-1]
	s := [2]int{last.Header.Teams[0].StartingScore, last.Header.Teams[1].StartingScore}
	if w := last.Round.Winner; w == 0 || w == 1 {
		s[w]++
	}
	return s
}

// Assemble groups round files into matches. Files may be given in any order.
// Only custom games count (see rec.Header.CustomGame), and a match recorded
// by several players counts once.
func Assemble(files []RoundFile) []*Match {
	var custom []RoundFile
	for _, f := range files {
		if f.Header.CustomGame() {
			custom = append(custom, f)
		}
	}
	files = custom
	// Header timestamps are zero-padded, so they sort as text; rounds of one
	// session share a folder and sort by file name.
	sort.SliceStable(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.Header.Timestamp != b.Header.Timestamp {
			return a.Header.Timestamp < b.Header.Timestamp
		}
		return a.Path < b.Path
	})
	files = dedupe(files)

	var matches []*Match
	var cur *Match
	for _, f := range files {
		if cur == nil || !cur.accept(f) {
			cur = &Match{
				MapID: f.Header.MapID,
				Map:   f.Header.Map,
				Teams: [2]string{f.Header.Teams[0].Name, f.Header.Teams[1].Name},
			}
			matches = append(matches, cur)
			cur.Rounds = append(cur.Rounds, f)
		}
	}
	return matches
}

// dedupe keeps one recording of each round when several players of a match
// sent theirs: their files differ, so storage cannot tell they are the same.
// A match keeps the rounds of the player who recorded the most complete
// ones; another player's recording fills a round they lack only when it
// names the teams the same way (opponents see "YOUR TEAM" and "ENEMY TEAM"
// swapped).
func dedupe(files []RoundFile) []RoundFile {
	recorder := func(h rec.Header) string {
		if h.RecordingProfileID != "" {
			return h.RecordingProfileID
		}
		return h.RecordingPlayerID
	}
	type tally struct {
		complete map[string]int
		first    []string // recorders in the order their files come
	}
	byMatch := map[string]*tally{}
	for _, f := range files {
		id := f.Header.MatchID
		if id == "" {
			continue
		}
		t := byMatch[id]
		if t == nil {
			t = &tally{complete: map[string]int{}}
			byMatch[id] = t
		}
		r := recorder(f.Header)
		if _, ok := t.complete[r]; !ok {
			t.first = append(t.first, r)
			t.complete[r] = 0
		}
		if f.Round != nil && f.Round.Complete {
			t.complete[r]++
		}
	}
	primary := map[string]string{}
	for id, t := range byMatch {
		best := t.first[0]
		for _, r := range t.first[1:] {
			if t.complete[r] > t.complete[best] {
				best = r
			}
		}
		primary[id] = best
	}
	type roundKey struct {
		match string
		round int
	}
	covered := map[roundKey]bool{}
	names := map[string][2]string{}
	for _, f := range files {
		id := f.Header.MatchID
		if id != "" && recorder(f.Header) == primary[id] {
			covered[roundKey{id, f.Header.RoundNumber}] = true
			names[id] = [2]string{f.Header.Teams[0].Name, f.Header.Teams[1].Name}
		}
	}
	var kept []RoundFile
	for _, f := range files {
		id := f.Header.MatchID
		switch {
		case id == "" || recorder(f.Header) == primary[id]:
		case covered[roundKey{id, f.Header.RoundNumber}]:
			continue
		case names[id] != [2]string{f.Header.Teams[0].Name, f.Header.Teams[1].Name}:
			continue
		default:
			covered[roundKey{id, f.Header.RoundNumber}] = true
		}
		kept = append(kept, f)
	}
	return kept
}

// accept adds f to the match if it continues it, and reports whether it did.
func (m *Match) accept(f RoundFile) bool {
	h := f.Header
	if h.MapID != m.MapID || h.Teams[0].Name != m.Teams[0] || h.Teams[1].Name != m.Teams[1] {
		return false
	}
	last := m.Rounds[len(m.Rounds)-1]
	switch n := h.RoundNumber; {
	case n > last.Header.RoundNumber:
		m.Rounds = append(m.Rounds, f)
		return true
	case n == last.Header.RoundNumber && sameStart(h, last.Header):
		// Same round from the same score: the earlier attempt did not count.
		m.Superseded = append(m.Superseded, last)
		m.Rounds[len(m.Rounds)-1] = f
		return true
	}
	// An earlier round number means the teams started the map over.
	return false
}

// sameStart reports whether two recordings start from the same teams and
// score.
func sameStart(a, b rec.Header) bool {
	for i := range a.Teams {
		if a.Teams[i].Name != b.Teams[i].Name || a.Teams[i].StartingScore != b.Teams[i].StartingScore {
			return false
		}
	}
	return true
}

// Roster returns the players of round i. A recording made right after a
// rehost can list a single team in its header; the other one is then taken
// from the nearest round that has it, without operators since those change
// every round.
func (m *Match) Roster(i int) []rec.Player {
	own := m.Rounds[i].Header.Players
	var has [2]bool
	for _, p := range own {
		if p.Team == 0 || p.Team == 1 {
			has[p.Team] = true
		}
	}
	if has[0] == has[1] {
		return own
	}
	missing := 0
	if has[0] {
		missing = 1
	}
	side := rec.Attack
	if own[0].Side == rec.Attack {
		side = rec.Defense
	}
	roster := append([]rec.Player(nil), own...)
	for d := 1; d < len(m.Rounds); d++ {
		for _, j := range []int{i - d, i + d} {
			if j < 0 || j >= len(m.Rounds) {
				continue
			}
			found := false
			for _, p := range m.Rounds[j].Header.Players {
				if p.Team == missing {
					roster = append(roster, rec.Player{ID: p.ID, ProfileID: p.ProfileID, Name: p.Name, Team: missing, Side: side})
					found = true
				}
			}
			if found {
				return roster
			}
		}
	}
	return roster
}
