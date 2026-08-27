package audit_inspection_reset_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"seismocal/internal/model"
	"seismocal/internal/storage"
)

// TestAppendAuditPreservesBrokenInspection verifies that appending a new event
// cannot hide an already-corrupted persisted audit chain after a restart.
func TestAppendAuditPreservesBrokenInspection(t *testing.T) {
	dir := t.TempDir()
	caseID := "CASE-AUDIT-1"
	st := storage.New(dir)
	st.AppendAudit(caseID, "case_created", map[string]string{"source": "test"})

	auditPath := filepath.Join(dir, caseID+".audit")
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	var events []model.AuditEvent
	if err := json.Unmarshal(raw, &events); err != nil || len(events) != 1 {
		t.Fatalf("decode audit: %v", err)
	}
	events[0].Digest = "tampered-digest"
	tampered, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("encode audit: %v", err)
	}
	if err := os.WriteFile(auditPath, tampered, 0644); err != nil {
		t.Fatalf("tamper audit: %v", err)
	}

	restarted := storage.New(dir)
	if inspection := restarted.AuditInspection(caseID); inspection.Healthy {
		t.Fatalf("expected restart to detect broken audit chain")
	}
	restarted.AppendAudit(caseID, "follow_up", nil)
	if inspection := restarted.AuditInspection(caseID); inspection.Healthy {
		t.Fatalf("%s: append reset broken audit inspection to healthy", t.Name())
	}
	if storage.VerifyAuditChain(restarted.Audit(caseID)) {
		t.Fatalf("%s: appended chain unexpectedly verifies after tampering", t.Name())
	}
}
