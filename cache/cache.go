// Package cache keeps decoded round files on disk, so each replay is decoded
// once. Decoding a round means decompressing tens of megabytes; reading it
// back from the cache takes a millisecond.
//
// Entries are keyed by the SHA-256 of the replay file, so a replay copied or
// moved elsewhere is still found, and tagged with rec.Version: an entry
// written by another version of the decoder is ignored and replaced.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pdrillin/r6-stats/match"
	"github.com/pdrillin/r6-stats/rec"
)

// Cache is a folder of decoded rounds. A nil *Cache decodes every file and
// stores nothing.
type Cache struct {
	dir string
}

// entry is one cached round. Header.Properties is not kept: nothing past
// decoding needs it.
type entry struct {
	Version int        `json:"version"`
	Header  rec.Header `json:"header"`
	Round   *rec.Round `json:"round"`
}

// DefaultDir is the cache folder of the user: %LocalAppData%\r6-stats on
// Windows, ~/.cache/r6-stats on Linux.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "r6-stats"), nil
}

// Open returns the cache stored in dir, creating the folder if needed.
func Open(dir string) (*Cache, error) {
	if err := os.MkdirAll(filepath.Join(dir, "rounds"), 0o755); err != nil {
		return nil, err
	}
	return &Cache{dir: dir}, nil
}

// Dir is the folder of the cache.
func (c *Cache) Dir() string { return c.dir }

func (c *Cache) path(sum string) string {
	return filepath.Join(c.dir, "rounds", sum+".json")
}

// Round returns the decoded round file at path, from the cache when it holds
// it, and reports whether it did.
func (c *Cache) Round(path string) (match.RoundFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return match.RoundFile{}, false, err
	}
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:])

	if c != nil {
		if e, ok := c.read(key); ok {
			return match.RoundFile{Path: path, Header: e.Header, Round: e.Round}, true, nil
		}
	}

	f, err := rec.Parse(data)
	if err != nil {
		return match.RoundFile{}, false, fmt.Errorf("%s: %w", path, err)
	}
	r, err := f.Round()
	if err != nil {
		return match.RoundFile{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if c != nil {
		// A cache that cannot be written only costs time.
		_ = c.write(key, entry{Version: rec.Version, Header: f.Header, Round: r})
	}
	return match.RoundFile{Path: path, Header: f.Header, Round: r}, false, nil
}

// read returns the entry for key when there is one, readable and written by
// this version of the decoder.
func (c *Cache) read(key string) (entry, bool) {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return entry{}, false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || e.Version != rec.Version || e.Round == nil {
		return entry{}, false
	}
	return e, true
}

// write stores an entry through a temporary file, so a reader never sees a
// partial entry.
func (c *Cache) write(key string, e entry) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(c.dir, "rounds"), key+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), c.path(key)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Stats tells how a load went.
type Stats struct {
	Cached  int // rounds read from the cache
	Decoded int // rounds decoded from their replay
}

// Rounds returns the round files at paths, in the same order. Files missing
// from the cache are decoded in parallel. The first error stops the load.
func (c *Cache) Rounds(paths []string) ([]match.RoundFile, Stats, error) {
	out := make([]match.RoundFile, len(paths))
	hits := make([]bool, len(paths))
	errs := make([]error, len(paths))

	// Each decode holds a whole decompressed body (up to ~100 MB), so the
	// number of workers is capped to keep memory in check.
	workers := min(runtime.NumCPU(), 6, len(paths))
	next := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				out[i], hits[i], errs[i] = c.Round(paths[i])
			}
		}()
	}
	for i := range paths {
		next <- i
	}
	close(next)
	wg.Wait()

	var st Stats
	for i := range paths {
		if errs[i] != nil {
			return nil, st, errs[i]
		}
		if hits[i] {
			st.Cached++
		} else {
			st.Decoded++
		}
	}
	return out, st, nil
}

// Info describes what the cache holds.
type Info struct {
	Entries int
	Bytes   int64
}

// Info counts the entries of the cache and their size.
func (c *Cache) Info() (Info, error) {
	var info Info
	files, err := os.ReadDir(filepath.Join(c.dir, "rounds"))
	if err != nil {
		return info, err
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		fi, err := f.Info()
		if err != nil {
			continue
		}
		info.Entries++
		info.Bytes += fi.Size()
	}
	return info, nil
}

// Clear removes every entry.
func (c *Cache) Clear() error {
	dir := filepath.Join(c.dir, "rounds")
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, f := range files {
		if err := os.Remove(filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}
