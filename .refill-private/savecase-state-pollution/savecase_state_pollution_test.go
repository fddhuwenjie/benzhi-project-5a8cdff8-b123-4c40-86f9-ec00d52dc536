package savecasestatepollution

import (
	"os"
	"path/filepath"
	"testing"

	"seismocal/internal/case_service"
	"seismocal/internal/storage"
)

func TestSaveCaseErrorDoesNotPolluteState(t *testing.T) {
	dir := t.TempDir()
	// Use a regular file as the storage directory so SaveCase's filesystem
	// write fails deterministically.
	badDir := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(badDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	st := storage.New(badDir)
	cs := case_service.New(st)
	if _, err := cs.Create("CASE-1", "STA-1", "Model", "SER-1", "eng", "STD-1"); err == nil {
		t.Fatal("expected persistence error")
	}
	if _, ok := st.GetCase("CASE-1"); ok {
		t.Fatal("case remained in memory after SaveCase persistence failure")
	}
}
