package rec

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// corpusDir holds real replays. They are large and not distributed with the
// code, so the corpus tests skip when the folder is missing.
const corpusDir = "../Replays"

func corpus(t *testing.T) []string {
	t.Helper()
	if _, err := os.Stat(corpusDir); err != nil {
		t.Skipf("no replay corpus at %s", corpusDir)
	}
	var paths []string
	err := filepath.WalkDir(corpusDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".rec") {
			paths = append(paths, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Skipf("no .rec file under %s", corpusDir)
	}
	return paths
}

func TestCorpusHeaders(t *testing.T) {
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		h := f.Header
		name := filepath.Base(p)
		if h.GameVersion == "" || h.MatchID == "" || h.MapID == "" {
			t.Errorf("%s: missing version, match id or map id: %+v", name, h)
		}
		if h.Teams[0].Name == "" || h.Teams[1].Name == "" {
			t.Errorf("%s: missing team name", name)
		}
		// The folder two levels up is named "<n> - <map>" in the corpus.
		if folder := filepath.Base(filepath.Dir(filepath.Dir(p))); h.Map == "" ||
			!strings.Contains(strings.ToLower(folder), strings.ToLower(strings.Fields(h.Map)[0])) {
			t.Errorf("%s: map %q (id %s) does not match folder %q", name, h.Map, h.MapID, folder)
		}
		// Aborted rounds only list one roster; complete ones list both.
		if n := len(h.Players); n != 5 && n != 10 {
			t.Errorf("%s: %d players", name, n)
		}
		sides := map[int]Side{}
		for _, pl := range h.Players {
			if pl.Name == "" || pl.Operator == "" || pl.Side == "" {
				t.Errorf("%s: incomplete player %+v", name, pl)
			}
			if s, ok := sides[pl.Team]; ok && s != pl.Side {
				t.Errorf("%s: team %d has players on both sides", name, pl.Team)
			}
			sides[pl.Team] = pl.Side
		}
		if len(sides) == 2 && sides[0] == sides[1] {
			t.Errorf("%s: both teams on side %q", name, sides[0])
		}
	}
}

func TestCorpusBodies(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	for _, p := range corpus(t) {
		f, err := Open(p)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		// WriteBody checks every chunk against the size stored in its header.
		if err := f.WriteBody(io.Discard); err != nil {
			t.Errorf("%s: %v", filepath.Base(p), err)
		}
	}
}

// TestCorpusRounds checks decoded rounds against the one fact the headers
// give for free: the score at the start of the next round of the same
// recording tells who won the previous one.
func TestCorpusRounds(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	paths := corpus(t)
	sort.Strings(paths)
	type decoded struct {
		path string
		h    Header
		r    *Round
	}
	var all []decoded
	for _, p := range paths {
		f, err := Open(p)
		if err != nil {
			t.Fatal(err)
		}
		r, err := f.Round()
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(p), err)
		}
		all = append(all, decoded{p, f.Header, r})
	}

	checked := 0
	sites := map[string][]string{} // by map id
	for i, d := range all {
		name := filepath.Base(d.path)
		known := map[string]bool{}
		for _, p := range d.h.Players {
			known[p.Name] = true
		}
		victims := map[string]bool{}
		for _, k := range d.r.Kills {
			if victims[k.Victim] {
				t.Errorf("%s: %s dies twice", name, k.Victim)
			}
			victims[k.Victim] = true
			if len(d.h.Players) == 10 && (!known[k.Victim] || (k.Killer != "" && !known[k.Killer])) {
				t.Errorf("%s: kill %q -> %q names a player missing from the header", name, k.Killer, k.Victim)
			}
			if k.Phase == PhasePrep || k.RemainingMs <= 0 {
				t.Errorf("%s: kill of %s has no usable time: %+v", name, k.Victim, k.Clock)
			}
		}
		if d.r.Site == "" || !strings.Contains(d.r.Site, " / ") {
			t.Errorf("%s: site %q", name, d.r.Site)
		}
		sites[d.h.MapID] = append(sites[d.h.MapID], d.r.Site)
		if !d.r.Complete {
			if d.r.Winner != -1 {
				t.Errorf("%s: incomplete round has winner %d", name, d.r.Winner)
			}
			continue
		}
		if d.r.Winner == -1 {
			t.Errorf("%s: complete round without a winner", name)
		}
		if i+1 == len(all) || filepath.Dir(all[i+1].path) != filepath.Dir(d.path) {
			continue // last round of the recording: nothing to compare with
		}
		next := all[i+1].h
		want := -1
		for team := range 2 {
			if next.Teams[team].StartingScore == d.h.Teams[team].StartingScore+1 {
				want = team
			}
		}
		if d.r.Winner != want {
			t.Errorf("%s: winner %d (%s), next round's score says %d", name, d.r.Winner, d.r.WinReason, want)
		}
		checked++
	}
	// A map has four bomb sites, of which a competitive pool uses three or
	// four; every site is defended at least once on each map of the corpus.
	for mapID, list := range sites {
		distinct := map[string]bool{}
		for _, s := range list {
			distinct[s] = true
		}
		if len(distinct) < 2 || len(distinct) > 4 {
			t.Errorf("map %s: %d distinct sites: %v", mapID, len(distinct), distinct)
		}
	}
	t.Logf("%d rounds decoded, %d winners checked against the next score", len(all), checked)
}

func TestParseRejectsBadInput(t *testing.T) {
	if _, err := Parse([]byte("not a replay")); err != ErrNotReplay {
		t.Errorf("wrong magic: got %v, want ErrNotReplay", err)
	}
	// A valid magic followed by nothing, then by a truncated header.
	for _, data := range [][]byte{
		[]byte("dissect\x00"),
		[]byte("dissect\x00\x08\x00\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff"),
	} {
		if _, err := Parse(data); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", data)
		}
	}
}
