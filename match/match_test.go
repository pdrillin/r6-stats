package match

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"r6-stats/rec"
)

// TestAssembleCorpus runs on the replay corpus, which is not distributed with
// the code. Two series were played: three maps, then four, several of them
// rehosted. The expected scores come from the decoder itself and guard
// against regressions; every round but the last of each recording is also
// checked against the game's own score in the rec package tests.
func TestAssembleCorpus(t *testing.T) {
	const dir = "../Replays"
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("no replay corpus at %s", dir)
	}
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	var files []RoundFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".rec") {
			return err
		}
		f, err := rec.Open(p)
		if err != nil {
			return err
		}
		r, err := f.Round()
		if err != nil {
			return err
		}
		files = append(files, RoundFile{Path: p, Header: f.Header, Round: r})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		mapName    string
		score      [2]int
		superseded int
	}{
		{"Nighthaven Labs", [2]int{3, 7}, 3},
		{"Kafe Dostoyevsky", [2]int{8, 7}, 0},
		{"Chalet", [2]int{7, 0}, 0},
		{"Bank", [2]int{3, 7}, 0},
		{"Border", [2]int{5, 7}, 0},
		{"Fortress", [2]int{8, 6}, 1},
		{"Lair", [2]int{5, 7}, 0},
	}
	got := Assemble(files)
	if len(got) != len(want) {
		t.Fatalf("got %d matches, want %d", len(got), len(want))
	}
	for i, m := range got {
		w := want[i]
		if m.Map != w.mapName || m.Score() != w.score || len(m.Superseded) != w.superseded {
			t.Errorf("match %d: got %s %v with %d superseded, want %s %v with %d",
				i, m.Map, m.Score(), len(m.Superseded), w.mapName, w.score, w.superseded)
		}
		for n, rf := range m.Rounds {
			if rf.Header.RoundNumber != n {
				t.Errorf("%s: position %d holds round %d", m.Map, n, rf.Header.RoundNumber)
			}
			if !rf.Round.Complete {
				t.Errorf("%s: round %d counts but is incomplete", m.Map, n+1)
			}
		}
		for _, rf := range m.Superseded {
			if rf.Round.Complete {
				t.Errorf("%s: a complete round %d was superseded", m.Map, rf.Header.RoundNumber+1)
			}
		}
		for n := range m.Rounds {
			var size [2]int
			for _, p := range m.Roster(n) {
				if p.Side == "" {
					t.Errorf("%s: round %d: %s has no side", m.Map, n+1, p.Name)
				}
				size[p.Team]++
			}
			if size != [2]int{5, 5} {
				t.Errorf("%s: round %d: roster sizes %v", m.Map, n+1, size)
			}
		}
	}
}
