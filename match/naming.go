package match

import (
	"sort"
	"strings"

	"github.com/pdrillin/stratdata/rec"
)

// Replays recorded by a player name the teams from the player's point of
// view, whoever the opponent: every opponent would be one "ENEMY TEAM".
var genericNames = map[string]bool{"YOUR TEAM": true, "ENEMY TEAM": true}

// SameTeam is how many players two rosters must share to be the same team.
const SameTeam = 3

// Alias is a name a member gave to a team, with the players it had then: it
// names every roster sharing SameTeam of them, past and future matches alike.
type Alias struct {
	Name    string   `json:"name"`
	Players []string `json:"players"`
}

// NamedBy says where a team name comes from.
type NamedBy string

const (
	ByReplay  NamedBy = "replay"  // the name written in the replay
	ByTag     NamedBy = "tag"     // the clan tag most of its players carry
	ByPlayer  NamedBy = "player"  // a generic name and its most present player
	ByManager NamedBy = "manager" // an alias a member chose
)

// PlayerID identifies a player across replays: the Ubisoft profile when the
// replay has it, the name otherwise.
func PlayerID(p rec.Player) string {
	if p.ProfileID == "" || p.ProfileID == "00000000-0000-0000-0000-000000000000" {
		return "name:" + p.Name
	}
	return p.ProfileID
}

// side is one team in one match.
type side struct {
	match, team int
	name        string         // as written in the replays
	rounds      map[string]int // player id -> rounds played
	names       map[string]string
}

// NameTeams returns the matches with their teams named: replays recorded by
// a player call every opponent "ENEMY TEAM", so teams are told apart by
// their players instead. Rosters sharing SameTeam players are one team,
// named by an alias when one matches, else by the clan tag most of its
// players carry, else by the generic name and its most present player.
// Names written in the replays (spectator recordings) are kept unless an
// alias renames them. The matches given are not modified.
func NameTeams(ms []*Match, aliases []Alias) ([]*Match, map[string]NamedBy) {
	var sides []*side
	for i, m := range ms {
		for t := 0; t < 2; t++ {
			sides = append(sides, &side{match: i, team: t, name: m.Teams[t], rounds: map[string]int{}, names: map[string]string{}})
		}
		for r := range m.Rounds {
			for _, p := range m.Roster(r) {
				if p.Team == 0 || p.Team == 1 {
					s := sides[2*i+p.Team]
					id := PlayerID(p)
					s.rounds[id]++
					s.names[id] = p.Name
				}
			}
		}
	}

	// Group the sides into teams: by name when the replay names them, by
	// shared players when it does not.
	parent := make([]int, len(sides))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	byName := map[string]int{}
	for i, s := range sides {
		if genericNames[s.name] {
			continue
		}
		if j, ok := byName[s.name]; ok {
			parent[find(i)] = find(j)
		} else {
			byName[s.name] = i
		}
	}
	for i, a := range sides {
		if !genericNames[a.name] {
			continue
		}
		for j := i + 1; j < len(sides); j++ {
			if b := sides[j]; genericNames[b.name] && shared(a.rounds, b.rounds) >= SameTeam {
				parent[find(i)] = find(j)
			}
		}
	}
	groups := map[int][]*side{}
	var roots []int
	for i, s := range sides {
		r := find(i)
		if groups[r] == nil {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], s)
	}

	names := make([][2]string, len(ms))
	by := map[string]NamedBy{}
	for _, r := range roots {
		g := groups[r]
		rounds := map[string]int{}
		pseudo := map[string]string{}
		written := map[string]int{}
		for _, s := range g {
			for id, n := range s.rounds {
				rounds[id] += n
				pseudo[id] = s.names[id]
			}
			written[s.name]++
		}
		name, how := teamName(rounds, pseudo, written, aliases)
		for _, s := range g {
			names[s.match][s.team] = name
		}
		by[name] = how
	}

	out := make([]*Match, len(ms))
	for i, m := range ms {
		c := *m
		c.Teams = names[i]
		c.Rounds = renamed(m.Rounds, names[i])
		c.Superseded = renamed(m.Superseded, names[i])
		out[i] = &c
	}
	return out, by
}

func renamed(files []RoundFile, names [2]string) []RoundFile {
	if files == nil {
		return nil
	}
	out := make([]RoundFile, len(files))
	for i, f := range files {
		f.Header.Teams[0].Name, f.Header.Teams[1].Name = names[0], names[1]
		out[i] = f
	}
	return out
}

func shared(a, b map[string]int) int {
	n := 0
	for id := range a {
		if _, ok := b[id]; ok {
			n++
		}
	}
	return n
}

// teamName names one team from its players (id -> rounds, id -> pseudo) and
// the names the replays wrote for it.
func teamName(rounds map[string]int, pseudo map[string]string, written map[string]int, aliases []Alias) (string, NamedBy) {
	best, bestShared := "", 0
	for _, a := range aliases {
		set := map[string]int{}
		for _, id := range a.Players {
			set[id] = 1
		}
		if n := shared(rounds, set); n >= SameTeam && n > bestShared {
			best, bestShared = a.Name, n
		}
	}
	if best != "" {
		return best, ByManager
	}

	base := ""
	for n, c := range written {
		if base == "" || c > written[base] || (c == written[base] && n < base) {
			base = n
		}
	}
	if !genericNames[base] {
		return base, ByReplay
	}
	if tag := commonTag(pseudo); tag != "" {
		return tag, ByTag
	}
	ids := make([]string, 0, len(rounds))
	for id := range rounds {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if rounds[a] != rounds[b] {
			return rounds[a] > rounds[b]
		}
		return strings.ToLower(pseudo[a]) < strings.ToLower(pseudo[b])
	})
	if len(ids) == 0 {
		return base, ByReplay
	}
	return base + " · " + pseudo[ids[0]], ByPlayer
}

// commonTag returns the clan tag ("Finn.OGLA" carries OGLA) that at least
// SameTeam players carry, when no other tag is as common.
func commonTag(pseudo map[string]string) string {
	count := map[string]int{}
	spelling := map[string]string{}
	for _, name := range pseudo {
		i := strings.LastIndex(name, ".")
		if i < 0 {
			continue
		}
		tag := name[i+1:]
		if !isTag(tag) {
			continue
		}
		key := strings.ToUpper(tag)
		count[key]++
		spelling[key] = tag
	}
	best, tie := "", false
	for k, n := range count {
		switch {
		case best == "" || n > count[best]:
			best, tie = k, false
		case n == count[best]:
			tie = true
		}
	}
	if best == "" || tie || count[best] < SameTeam {
		return ""
	}
	return spelling[best]
}

func isTag(s string) bool {
	if len(s) < 2 || len(s) > 8 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
