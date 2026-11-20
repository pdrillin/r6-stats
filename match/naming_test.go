package match

import (
	"testing"

	"github.com/pdrillin/r6-stats/rec"
)

// playerMatch builds a one-round match between two rosters, named as a player's
// replay names them.
func playerMatch(id string, own, opp []string) *Match {
	var players []rec.Player
	for _, n := range own {
		players = append(players, rec.Player{Name: n, ProfileID: "p-" + n, Team: 0})
	}
	for _, n := range opp {
		players = append(players, rec.Player{Name: n, ProfileID: "p-" + n, Team: 1})
	}
	h := rec.Header{MatchID: id, Teams: [2]rec.Team{{Name: "YOUR TEAM"}, {Name: "ENEMY TEAM"}}, Players: players}
	return &Match{Teams: [2]string{"YOUR TEAM", "ENEMY TEAM"}, Rounds: []RoundFile{{Header: h, Round: &rec.Round{}}}}
}

func TestNameTeams(t *testing.T) {
	us := []string{"Pepsi.RCK", "Nivalem.RCK", "Capy..RCK", "Universe.TEST", "W4rthox"}
	ogla := []string{"Finn.OGLA", "Paulo.OGLA", "E3xA.OGLA", "NoVa.OGLA", "Oko..."}
	ogla2 := []string{"Finn.OGLA", "Paulo.OGLA", "E3xA.OGLA", "Sub.X", "Other"} // three regulars
	mix := []string{"YoZaX.pht", "salva.Alone", "Tounsi-", "SqualX.DVB", "Shikai.MSF"}
	ms := []*Match{
		playerMatch("1", us, ogla),
		playerMatch("2", us, ogla2),
		playerMatch("3", us, mix),
	}
	named, by := NameTeams(ms, nil)
	want := [][2]string{{"RCK", "OGLA"}, {"RCK", "OGLA"}, {"RCK", "ENEMY TEAM · salva.Alone"}}
	for i, m := range named {
		if m.Teams != want[i] {
			t.Errorf("match %d: %v, want %v", i+1, m.Teams, want[i])
		}
		if n := m.Rounds[0].Header.Teams[1].Name; n != want[i][1] {
			t.Errorf("match %d: round header names %q", i+1, n)
		}
	}
	if by["OGLA"] != ByTag || by["ENEMY TEAM · salva.Alone"] != ByPlayer {
		t.Errorf("named by %v", by)
	}
	if ms[0].Teams[1] != "ENEMY TEAM" || ms[0].Rounds[0].Header.Teams[1].Name != "ENEMY TEAM" {
		t.Error("the matches given were modified")
	}

	// A manager's name follows the players into a new match.
	alias := Alias{Name: "Les mixeurs", Players: []string{"p-YoZaX.pht", "p-salva.Alone", "p-Tounsi-"}}
	named, by = NameTeams(append(ms, playerMatch("4", us, mix[:3])), []Alias{alias})
	if named[2].Teams[1] != "Les mixeurs" || named[3].Teams[1] != "Les mixeurs" || by["Les mixeurs"] != ByManager {
		t.Errorf("alias: %v %v %v", named[2].Teams, named[3].Teams, by)
	}

	// Names written by a spectator's replay are kept.
	spec := playerMatch("5", us, ogla)
	spec.Teams = [2]string{"FUR", "FAZE"}
	if named, _ := NameTeams([]*Match{spec}, nil); named[0].Teams != spec.Teams {
		t.Errorf("spectator names changed: %v", named[0].Teams)
	}
}
