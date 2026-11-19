package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "data.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.MyTeam() != "" || s.Kind("m1") != DefaultKind {
		t.Fatalf("empty store: team %q, kind %q", s.MyTeam(), s.Kind("m1"))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("opening created the file")
	}

	if err := s.SetMyTeam("FAZE"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind("m1", Official); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind("m2", Pracc); err != nil {
		t.Fatal(err)
	}
	if err := s.SetKind("m3", "ranked"); err == nil {
		t.Error("unknown kind accepted")
	}

	// Everything survives a reopen.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.MyTeam() != "FAZE" || s.Kind("m1") != Official || s.Kind("m2") != Pracc || s.Kind("m3") != DefaultKind {
		t.Errorf("reopened: team %q, kinds %q %q %q", s.MyTeam(), s.Kind("m1"), s.Kind("m2"), s.Kind("m3"))
	}
	if err := s.SetMyTeam(""); err != nil {
		t.Fatal(err)
	}
	if s, _ := Open(path); s.MyTeam() != "" {
		t.Errorf("team not cleared: %q", s.MyTeam())
	}

	// No temporary file is left behind.
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Errorf("%d files next to the store, want 1", len(files))
	}
}

func TestOpenBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, err := Open(path); err == nil {
		t.Error("broken file opened as an empty store")
	}
}

func TestFailedSaveKeepsMemory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder permissions")
	}
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	// A read-only folder: saving fails.
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	if err := s.SetKind("m1", Official); err == nil {
		t.Fatal("save into a read-only folder succeeded")
	}
	if err := s.SetMyTeam("FAZE"); err == nil {
		t.Fatal("save into a read-only folder succeeded")
	}
	if s.Kind("m1") != DefaultKind || s.MyTeam() != "" {
		t.Errorf("failed saves changed the store: %q %q", s.Kind("m1"), s.MyTeam())
	}
}
