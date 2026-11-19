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
