package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte(`# comment
STRATDATA_TEST_A=plain value
export STRATDATA_TEST_B="quoted # not a comment"
STRATDATA_TEST_C='single'
STRATDATA_TEST_SET=from file

STRATDATA_TEST_EMPTY=
`), 0o600)
	t.Setenv("STRATDATA_TEST_SET", "from shell")
	for _, k := range []string{"STRATDATA_TEST_A", "STRATDATA_TEST_B", "STRATDATA_TEST_C", "STRATDATA_TEST_EMPTY"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"STRATDATA_TEST_A":     "plain value",
		"STRATDATA_TEST_B":     "quoted # not a comment",
		"STRATDATA_TEST_C":     "single",
		"STRATDATA_TEST_SET":   "from shell",
		"STRATDATA_TEST_EMPTY": "",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if err := loadEnvFile(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Errorf("missing file: %v", err)
	}
	os.WriteFile(path, []byte("no equals sign\n"), 0o600)
	if err := loadEnvFile(path); err == nil {
		t.Error("malformed line accepted")
	}
}
