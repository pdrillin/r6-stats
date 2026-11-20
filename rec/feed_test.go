package rec

import "testing"

func TestAlignFeedTeams(t *testing.T) {
	h := &Header{Players: []Player{
		{Name: "a", Team: 0}, {Name: "b", Team: 0},
		{Name: "x", Team: 1}, {Name: "y", Team: 1},
	}}
	// A player's replay: the recording player's team (header team 1) is
	// feed team 0.
	kills := []Kill{
		{Killer: "x", KillerTeam: 0, Victim: "a", VictimTeam: 1},
		{Killer: "b", KillerTeam: 1, Victim: "y", VictimTeam: 0},
		{Killer: "", KillerTeam: -1, Victim: "unknown", VictimTeam: 1},
	}
	alignFeedTeams(h, kills)
	want := [][2]int{{1, 0}, {0, 1}, {-1, 0}}
	for i, k := range kills {
		if got := [2]int{k.KillerTeam, k.VictimTeam}; got != want[i] {
			t.Errorf("kill %d: teams %v, want %v", i, got, want[i])
		}
	}

	// A spectator's replay already numbers teams as the header does.
	kills = []Kill{{Killer: "a", KillerTeam: 0, Victim: "x", VictimTeam: 1}}
	alignFeedTeams(h, kills)
	if k := kills[0]; k.KillerTeam != 0 || k.VictimTeam != 1 {
		t.Errorf("spectator feed changed: %+v", k)
	}
}
