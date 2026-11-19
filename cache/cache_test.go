package cache

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
)

const corpusDir = "../Replays"

// corpus returns the replays of the corpus, or skips the test without one.
func corpus(t *testing.T) []string {
	t.Helper()
	var paths []string
	filepath.WalkDir(corpusDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".rec") {
			paths = append(paths, p)
		}
		return nil
	})
	if len(paths) == 0 {
		t.Skipf("no replay corpus at %s", corpusDir)
	}
	return paths
}

// decode reads a round file without any cache, as the cache must return it.
func decode(t *testing.T, path string) match.RoundFile {
	t.Helper()
	f, err := rec.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.Round()
	if err != nil {
		t.Fatal(err)
	}
	h := f.Header
	h.Properties = nil // not cached
	return match.RoundFile{Path: path, Header: h, Round: r}
}

func open(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func round(t *testing.T, c *Cache, path string) (match.RoundFile, bool) {
	t.Helper()
	rf, hit, err := c.Round(path)
	if err != nil {
		t.Fatal(err)
	}
	rf.Header.Properties = nil
	return rf, hit
}

func TestRoundCached(t *testing.T) {
	path := corpus(t)[0]
	want := decode(t, path)
	c := open(t)

	got, hit := round(t, c, path)
	if hit {
		t.Error("first read was a cache hit")
	}
	if !reflect.DeepEqual(got, want) {
		t.Error("decoded round differs from rec.Open")
	}
	got, hit = round(t, c, path)
	if !hit {
		t.Error("second read missed the cache")
	}
	if !reflect.DeepEqual(got, want) {
		t.Error("cached round differs from the decoded one")
	}

	// The key is the content: a copy elsewhere is found.
	data, _ := os.ReadFile(path)
	copied := filepath.Join(t.TempDir(), "copy.rec")
	os.WriteFile(copied, data, 0o644)
	got, hit = round(t, c, copied)
	if !hit || got.Path != copied {
		t.Errorf("copy: hit %v, path %q", hit, got.Path)
	}
}

func TestStaleOrBrokenEntries(t *testing.T) {
	path := corpus(t)[0]
	want := decode(t, path)
	c := open(t)
	round(t, c, path)
	entries, _ := filepath.Glob(filepath.Join(c.Dir(), "rounds", "*.json"))
	if len(entries) != 1 {
		t.Fatalf("%d entries, want 1", len(entries))
	}
	entryPath := entries[0]

	var e entry
	b, _ := os.ReadFile(entryPath)
	json.Unmarshal(b, &e)
	e.Version = rec.Version - 1
	old, _ := json.Marshal(e)

	for name, content := range map[string][]byte{
		"other version": old,
		"truncated":     b[:len(b)/2],
		"empty":         {},
	} {
		os.WriteFile(entryPath, content, 0o644)
		got, hit := round(t, c, path)
		if hit {
			t.Errorf("%s: entry used", name)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: round differs", name)
		}
		// The entry was rewritten.
		if _, hit := round(t, c, path); !hit {
			t.Errorf("%s: entry not replaced", name)
		}
	}
}

func TestNilCache(t *testing.T) {
	path := corpus(t)[0]
	var c *Cache
	got, hit := round(t, c, path)
	if hit || !reflect.DeepEqual(got, decode(t, path)) {
		t.Errorf("nil cache: hit %v or round differs", hit)
	}
}

func TestRounds(t *testing.T) {
	paths := corpus(t)
	if len(paths) > 6 {
		paths = paths[:6]
	}
	c := open(t)
	first, st, err := c.Rounds(paths)
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{Decoded: len(paths)}) {
		t.Errorf("first load: %+v", st)
	}
	second, st, err := c.Rounds(paths)
	if err != nil {
		t.Fatal(err)
	}
	if st != (Stats{Cached: len(paths)}) {
		t.Errorf("second load: %+v", st)
	}
	for i := range paths {
		if first[i].Path != paths[i] || second[i].Path != paths[i] {
			t.Errorf("order: %s, %s, want %s", first[i].Path, second[i].Path, paths[i])
		}
	}

	info, err := c.Info()
	if err != nil || info.Entries != len(paths) || info.Bytes == 0 {
		t.Errorf("info = %+v, %v", info, err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if info, _ := c.Info(); info.Entries != 0 {
		t.Errorf("%d entries after Clear", info.Entries)
	}
}

func TestRoundsError(t *testing.T) {
	c := open(t)
	bad := filepath.Join(t.TempDir(), "bad.rec")
	os.WriteFile(bad, []byte("not a replay"), 0o644)
	for _, p := range []string{bad, filepath.Join(t.TempDir(), "missing.rec")} {
		if _, _, err := c.Rounds([]string{p}); err == nil {
			t.Errorf("%s: no error", filepath.Base(p))
		}
	}
	if info, _ := c.Info(); info.Entries != 0 {
		t.Errorf("a failed decode left %d entries", info.Entries)
	}
}

// TestCorpusRoundTrip checks that every round of the corpus comes back from
// the cache exactly as decoded.
func TestCorpusRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("decompresses the whole corpus")
	}
	paths := corpus(t)
	c := open(t)
	decoded, _, err := c.Rounds(paths)
	if err != nil {
		t.Fatal(err)
	}
	cached, st, err := c.Rounds(paths)
	if err != nil {
		t.Fatal(err)
	}
	if st.Cached != len(paths) {
		t.Fatalf("%d of %d rounds from the cache", st.Cached, len(paths))
	}
	for i := range paths {
		decoded[i].Header.Properties = nil
		if !reflect.DeepEqual(decoded[i], cached[i]) {
			t.Errorf("%s: cached round differs", filepath.Base(paths[i]))
		}
	}
}
