package match

import (
	"fmt"
	"testing"

	"github.com/pdrillin/stratdata/rec"
)

// round builds a recording of round n of match id by recorder, whose own
// team is header team own.
func round(id, recorder string, own, n int, complete bool, matchType string) RoundFile {
	names := [2]string{"ENEMY TEAM", "ENEMY TEAM"}
	names[own] = "YOUR TEAM"
	return RoundFile{
		Path: fmt.Sprintf("%s/%s-R%02d.rec", recorder, id, n+1),
		Header: rec.Header{
			Timestamp:          fmt.Sprintf("2026-11-16-21-%02d-00", n),
			MatchID:            id,
			MatchType:          matchType,
			MapID:              "407558616688",
			RoundNumber:        n,
			RecordingProfileID: recorder,
			Teams:              [2]rec.Team{{Name: names[0]}, {Name: names[1]}},
		},
		Round: &rec.Round{Complete: complete, Winner: 0},
	}
}

func TestAssembleOnlyCustomGames(t *testing.T) {
	files := []RoundFile{
		round("ranked", "me", 0, 0, true, "2"),
		round("arcade", "me", 0, 0, true, "13"),
		round("pracc", "me", 0, 0, true, "4"),
		round("official", "me", 0, 0, true, "3"),
	}
	for i := range files {
		files[i].Header.MapID = files[i].Header.MatchID // not taken for rehosts
	}
	got := Assemble(files)
	var ids []string
	for _, m := range got {
		ids = append(ids, m.Rounds[0].Header.MatchID)
	}
	if fmt.Sprint(ids) != "[official pracc]" {
		t.Errorf("got matches %v, want [official pracc]", ids)
	}
}

func TestAssembleSameMatchFromSeveralPlayers(t *testing.T) {
	var files []RoundFile
	// A teammate recorded rounds 1 to 3, I recorded 1 and 2 (my round 2 is
	// incomplete), the opponent recorded all three with teams swapped.
	for n := 0; n < 3; n++ {
		files = append(files, round("m", "mate", 1, n, true, "4"))
		files = append(files, round("m", "opponent", 0, n, true, "4"))
	}
	files = append(files, round("m", "me", 1, 0, true, "4"), round("m", "me", 1, 1, false, "4"))

	got := Assemble(files)
	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	m := got[0]
	if len(m.Rounds) != 3 || len(m.Superseded) != 0 {
		t.Fatalf("got %d rounds and %d superseded, want 3 and 0", len(m.Rounds), len(m.Superseded))
	}
	for i, rf := range m.Rounds {
		if rf.Header.RecordingProfileID != "mate" {
			t.Errorf("round %d comes from %s, want the most complete recorder", i+1, rf.Header.RecordingProfileID)
		}
	}

	// A round only another teammate recorded fills the gap; the opponent's,
	// whose team names are swapped, does not.
	files = []RoundFile{
		round("g", "me", 1, 0, true, "4"), round("g", "me", 1, 1, true, "4"),
		round("g", "opponent", 0, 2, true, "4"),
		round("g", "mate", 1, 3, true, "4"),
	}
	m = Assemble(files)[0]
	var from []string
	for _, rf := range m.Rounds {
		from = append(from, rf.Header.RecordingProfileID)
	}
	if fmt.Sprint(from) != "[me me mate]" {
		t.Errorf("rounds come from %v, want [me me mate]", from)
	}
}
