package rec

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusPositions checks decoded positions against what the rest of the
// replay says: every player is tracked, the room of nearly every kill is
// known, and defenders start the round on or next to their bomb site.
func TestCorpusPositions(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	var kills, killerRooms, victimRooms, rounds, siteStarts, checked int
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Round()
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(p)
		if !r.Complete {
			continue
		}
		rounds++
		for _, pl := range f.Header.Players {
			if len(r.Positions[pl.Name]) == 0 {
				t.Errorf("%s: no position for %s", name, pl.Name)
			}
		}
		for _, k := range r.Kills {
			kills++
			if k.VictimRoom != "" {
				victimRooms++
			}
			if k.Killer == "" || k.KillerRoom != "" {
				killerRooms++
			}
		}
		// Defenders spawn on the site, so one of them is usually in one of
		// its two rooms at the first reading. Not always: on Kafe, the
		// Fireplace Hall / Mining Room spawn is in the Train Museum next
		// door. Headers that list only the attackers are left out.
		siteRooms := strings.Split(r.Site, " / ")
		defenders, found := 0, false
		for _, pl := range f.Header.Players {
			if pl.Side != Defense {
				continue
			}
			defenders++
			if pos := r.Positions[pl.Name]; len(pos) > 0 {
				for _, room := range siteRooms {
					if pos[0].Room == room {
						found = true
					}
				}
			}
		}
		if defenders == 0 {
			continue
		}
		checked++
		if found {
			siteStarts++
		} else {
			t.Logf("%s: no defender starts on %s", name, r.Site)
		}
	}
	t.Logf("%d complete rounds, %d of %d checked with a defender starting on site", rounds, siteStarts, checked)
	t.Logf("%d kills: victim room known for %d, killer room for %d", kills, victimRooms, killerRooms)
	if victimRooms < kills*98/100 || killerRooms < kills*98/100 {
		t.Errorf("rooms known for too few kills")
	}
	if siteStarts < checked*9/10 {
		t.Errorf("defenders start on site in only %d of %d rounds", siteStarts, checked)
	}
}

// TestCorpusPlanter checks that every plant is credited to an attacker.
func TestCorpusPlanter(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	plants, credited := 0, 0
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Round()
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(p)
		if r.Plant == nil {
			if r.Planter != "" {
				t.Errorf("%s: planter %s without a plant", name, r.Planter)
			}
			continue
		}
		plants++
		if r.Planter == "" {
			t.Logf("%s: plant at %d ms without a planter", name, r.Plant.ElapsedMs)
			continue
		}
		credited++
		for _, pl := range f.Header.Players {
			if pl.Name == r.Planter && pl.Side != Attack {
				t.Errorf("%s: planter %s is not attacking", name, r.Planter)
			}
		}
	}
	t.Logf("%d plants, %d credited", plants, credited)
}

// TestCorpusScoreboards checks the scoreboard gains against the kill feed.
// Deaths must match for every player. Kills must match for every team, but
// not always for every player: when a player is downed by one enemy and
// finished by another, the feed names the finisher while the scoreboard can
// credit the kill to the first and an assist to the second.
func TestCorpusScoreboards(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	var players, sameKills, assists int
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Round()
		if err != nil {
			t.Fatal(err)
		}
		if !r.Complete {
			continue
		}
		name := filepath.Base(p)
		kills, deaths := map[string]int{}, map[string]int{}
		var teamFeed, teamBoard [2]int
		team := map[string]int{}
		for _, k := range r.Kills {
			deaths[k.Victim]++
			team[k.Victim] = k.VictimTeam
			if k.Killer != "" && k.KillerTeam != k.VictimTeam && (k.KillerTeam == 0 || k.KillerTeam == 1) {
				kills[k.Killer]++
				team[k.Killer] = k.KillerTeam
				teamFeed[k.KillerTeam]++
			}
		}
		for _, pl := range f.Header.Players {
			team[pl.Name] = pl.Team
		}
		for pl, sb := range r.Scoreboards {
			players++
			assists += sb.Assists
			if sb.Deaths != deaths[pl] {
				t.Errorf("%s: %s scoreboard %d deaths, feed %d", name, pl, sb.Deaths, deaths[pl])
			}
			if sb.Kills == kills[pl] {
				sameKills++
			}
			if tm, ok := team[pl]; ok && (tm == 0 || tm == 1) {
				teamBoard[tm] += sb.Kills
			}
			if sb.Assists < 0 || sb.Assists > 4 {
				t.Errorf("%s: %s has %d assists", name, pl, sb.Assists)
			}
		}
		if len(f.Header.Players) == 10 && teamFeed != teamBoard {
			t.Errorf("%s: team kills feed %v, scoreboard %v", name, teamFeed, teamBoard)
		}
	}
	t.Logf("%d player-rounds, %d with the same kills as the feed, %d assists", players, sameKills, assists)
}

// TestCorpusDefuser checks that every defuse is credited to a defender who
// is alive and next to the defuser when the round ends: in a site room, in
// the room the defuser was planted in (it can be planted at the edge of the
// site), or on the threshold of a site room.
func TestCorpusDefuser(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	defuses, credited := 0, 0
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Round()
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(p)
		if r.WinReason != DefuserDisabled {
			if r.Defuser != "" {
				t.Errorf("%s: defuser %s in a round won by %s", name, r.Defuser, r.WinReason)
			}
			continue
		}
		defuses++
		if r.Defuser == "" {
			t.Errorf("%s: defuse without a defuser", name)
			continue
		}
		credited++
		for _, k := range r.Kills {
			if k.Victim == r.Defuser {
				t.Errorf("%s: defuser %s died", name, r.Defuser)
			}
		}
		pos := r.Positions[r.Defuser]
		if len(pos) == 0 {
			t.Errorf("%s: no position for defuser %s", name, r.Defuser)
			continue
		}
		inSite := func(room string) bool {
			for _, s := range strings.Split(r.Site, " / ") {
				if s == room {
					return true
				}
			}
			return false
		}
		plantRoom := ""
		for _, q := range r.Positions[r.Planter] {
			if q.ElapsedMs <= r.Plant.ElapsedMs {
				plantRoom = q.Room
			}
		}
		room := pos[len(pos)-1].Room
		threshold := len(pos) > 1 && inSite(pos[len(pos)-2].Room)
		if !inSite(room) && room != plantRoom && !threshold {
			t.Errorf("%s: defuser %s ends in %s, site is %s, plant in %s", name, r.Defuser, room, r.Site, plantRoom)
		}
		t.Logf("%s: %s defuses in %s (plant in %s)", name, r.Defuser, room, plantRoom)
	}
	t.Logf("%d defuses, %d credited", defuses, credited)
}
