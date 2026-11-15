package httpserv

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestRecoverPending(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("1_done.mkv")
	write("1_done.mkv.finalize.lock")
	write("2_partial.mkv")
	write("2_partial.mkv.assemble.lock")
	write("2_partial.mkv_0.part")
	write("noprefix.mkv")

	s := &Server{tmpDir: dir}
	got := s.recoverPending()
	sort.Strings(got)

	if len(got) != 1 || got[0] != "1_done.mkv" {
		t.Fatalf("unexpected pending list: %v", got)
	}
	for _, gone := range []string{"1_done.mkv.finalize.lock", "2_partial.mkv", "2_partial.mkv.assemble.lock"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "2_partial.mkv_0.part")); err != nil {
		t.Errorf("part file must be kept: %v", err)
	}
}
