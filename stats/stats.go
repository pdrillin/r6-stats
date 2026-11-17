// Package stats derives player and team statistics from assembled matches.
package stats

import (
	"sort"

	"r6-stats/match"
	"r6-stats/rec"
)

// Options tunes the definitions that are a matter of convention.
type Options struct {
	// TradeWindowMs is how long after a death the killer must die for the
	// death to count as traded. Defaults to DefaultTradeWindowMs.
	TradeWindowMs int
}

// DefaultTradeWindowMs is the trade window used when none is given.
const DefaultTradeWindowMs = 5000

// Record counts how often something happened and how often it paid off.
type Record struct {
	Count int `json:"count"`
	Won   int `json:"won"`
}

func (r *Record) add(won bool) {
	r.Count++
	if won {
		r.Won++
	}
}

// Player holds the totals of one player.
type Player struct {
	Name   string `json:"name"`
	Team   string `json:"team"`
	Rounds int    `json:"rounds"`

	Kills     int `json:"kills"` // enemies only
	Deaths    int `json:"deaths"`
	Headshots int `json:"headshots"`

	// OpeningKills counts the rounds where the player got the first kill,
	// and how many of those rounds the team won. OpeningDeaths likewise for
	// dying first.
	OpeningKills  Record `json:"openingKills"`
	OpeningDeaths Record `json:"openingDeaths"`

	// TradeKills are kills on an enemy who had just killed a teammate.
	// TradedDeaths are deaths avenged that way by a teammate.
	TradeKills   int `json:"tradeKills"`
	TradedDeaths int `json:"tradedDeaths"`

	// Clutches counts the rounds where the player was the last one alive,
	// keyed by the number of enemies alive at that moment.
	Clutches map[int]*Record `json:"clutches"`
}

// Situation is a count of players alive on each side.
type Situation struct {
	Own   int `json:"own"`
	Enemy int `json:"enemy"`
}

// Team holds the totals of one team.
type Team struct {
	Name   string `json:"name"`
	Rounds Record `json:"rounds"`

	// OpeningKills counts the rounds where the team got the first kill;
	// OpeningDeaths the rounds where it lost the first player.
	OpeningKills  Record `json:"openingKills"`
	OpeningDeaths Record `json:"openingDeaths"`

	Deaths       int `json:"deaths"`
	TradedDeaths int `json:"tradedDeaths"`

	// Situations counts the rounds that went through each state, at most
	// once per round, and how many the team went on to win.
	Situations map[Situation]*Record `json:"-"`
}

// Report is the result of Compute.
type Report struct {
	Players []*Player `json:"players"` // by team, then kills
	Teams   []*Team   `json:"teams"`   // in order of appearance
	// Rounds is the number of rounds counted. SkippedRounds are those left
	// out because their winner is unknown.
	Rounds        int `json:"rounds"`
	SkippedRounds int `json:"skippedRounds"`
}

// Compute totals every round of the given matches. Players are matched
// across rounds by Ubisoft profile id when the replay has one, by name
// otherwise; teams by name.
func Compute(opt Options, matches ...*match.Match) *Report {
	if opt.TradeWindowMs <= 0 {
		opt.TradeWindowMs = DefaultTradeWindowMs
	}
	c := &collector{opt: opt, players: map[string]*Player{}, teams: map[string]*Team{}, rep: &Report{}}
	for _, m := range matches {
		for i, rf := range m.Rounds {
			if w := rf.Round.Winner; w != 0 && w != 1 {
				c.rep.SkippedRounds++
				continue
			}
			c.rep.Rounds++
			c.round(m.Teams, m.Roster(i), rf.Round)
		}
	}
	sort.SliceStable(c.rep.Players, func(i, j int) bool {
		a, b := c.rep.Players[i], c.rep.Players[j]
		if a.Team != b.Team {
			return c.teamOrder[a.Team] < c.teamOrder[b.Team]
		}
		if a.Kills != b.Kills {
			return a.Kills > b.Kills
		}
		return a.Name < b.Name
	})
	return c.rep
}

type collector struct {
	opt       Options
	rep       *Report
	players   map[string]*Player // by identity key
	teams     map[string]*Team
	teamOrder map[string]int
}

func (c *collector) team(name string) *Team {
	t := c.teams[name]
	if t == nil {
		t = &Team{Name: name, Situations: map[Situation]*Record{}}
		c.teams[name] = t
		if c.teamOrder == nil {
			c.teamOrder = map[string]int{}
		}
		c.teamOrder[name] = len(c.rep.Teams)
		c.rep.Teams = append(c.rep.Teams, t)
	}
	return t
}

const noProfile = "00000000-0000-0000-0000-000000000000"

func (c *collector) player(p rec.Player, team string) *Player {
	key := p.ProfileID
	if key == "" || key == noProfile {
		key = "name:" + p.Name
	}
	pl := c.players[key]
	if pl == nil {
		pl = &Player{Name: p.Name, Team: team, Clutches: map[int]*Record{}}
		c.players[key] = pl
		c.rep.Players = append(c.rep.Players, pl)
	}
	return pl
}

// round adds one round. teams are the two team names by index, roster the
// ten players, r the decoded round, whose winner is known.
func (c *collector) round(teams [2]string, roster []rec.Player, r *rec.Round) {
	team := [2]*Team{c.team(teams[0]), c.team(teams[1])}
	byName := map[string]*Player{}
	alive := [2]map[string]bool{{}, {}}
	for _, p := range roster {
		if p.Team != 0 && p.Team != 1 {
			continue
		}
		pl := c.player(p, teams[p.Team])
		pl.Rounds++
		byName[p.Name] = pl
		alive[p.Team][p.Name] = true
	}
	for t := range team {
		team[t].Rounds.add(r.Winner == t)
	}

	var clutching [2]bool
	for i, k := range r.Kills {
		vt := k.VictimTeam
		if vt != 0 && vt != 1 {
			continue
		}
		victim, killer := byName[k.Victim], byName[k.Killer]
		won := r.Winner == vt

		if victim != nil {
			victim.Deaths++
		}
		team[vt].Deaths++
		if enemy(k) && killer != nil {
			killer.Kills++
			if k.Headshot {
				killer.Headshots++
			}
		}

		if i == 0 {
			if victim != nil {
				victim.OpeningDeaths.add(won)
			}
			team[vt].OpeningDeaths.add(won)
			if enemy(k) {
				if killer != nil {
					killer.OpeningKills.add(!won)
				}
				team[1-vt].OpeningKills.add(!won)
			}
		}

		if avenger, ok := c.tradedBy(r.Kills, i); ok {
			if victim != nil {
				victim.TradedDeaths++
			}
			team[vt].TradedDeaths++
			// One kill can avenge several teammates; it is still one
			// trade kill, counted for the first of them.
			if a := byName[r.Kills[avenger].Killer]; a != nil && c.firstAvenged(r.Kills, avenger) == i {
				a.TradeKills++
			}
		}

		delete(alive[vt], k.Victim)
		n := [2]int{len(alive[0]), len(alive[1])}
		if n[0] == 0 || n[1] == 0 {
			continue
		}
		for t := range team {
			s := Situation{Own: n[t], Enemy: n[1-t]}
			sit := team[t].Situations[s]
			if sit == nil {
				sit = &Record{}
				team[t].Situations[s] = sit
			}
			sit.add(r.Winner == t)

			if n[t] == 1 && !clutching[t] {
				clutching[t] = true
				for name := range alive[t] {
					if pl := byName[name]; pl != nil {
						cl := pl.Clutches[n[1-t]]
						if cl == nil {
							cl = &Record{}
							pl.Clutches[n[1-t]] = cl
						}
						cl.add(r.Winner == t)
					}
				}
			}
		}
	}
}

// enemy reports whether the kill is one team killing a player of the other,
// as opposed to a team kill, a suicide or an uncredited death.
func enemy(k rec.Kill) bool {
	return k.Killer != "" && (k.KillerTeam == 0 || k.KillerTeam == 1) && k.KillerTeam == 1-k.VictimTeam
}

// tradedBy returns the index of the kill that avenges kills[i]: the first
// later kill, within the trade window, where the killer of kills[i] dies to
// the victim's team. Only enemy kills can be traded.
func (c *collector) tradedBy(kills []rec.Kill, i int) (int, bool) {
	k := kills[i]
	if !enemy(k) {
		return 0, false
	}
	for j := i + 1; j < len(kills); j++ {
		a := kills[j]
		if a.ElapsedMs-k.ElapsedMs > c.opt.TradeWindowMs {
			break
		}
		if a.Victim == k.Killer && enemy(a) && a.KillerTeam == k.VictimTeam {
			return j, true
		}
	}
	return 0, false
}

// firstAvenged returns the index of the earliest death that kills[j] avenges.
func (c *collector) firstAvenged(kills []rec.Kill, j int) int {
	for i := 0; i < j; i++ {
		if a, ok := c.tradedBy(kills, i); ok && a == j {
			return i
		}
	}
	return -1
}
