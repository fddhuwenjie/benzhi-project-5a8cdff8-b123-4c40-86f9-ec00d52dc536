package audit_snapshot_alias_pollution_test

import (
	"seismocal/internal/storage"
	"testing"
)

func TestAuditSnapshotMutationDoesNotPolluteStore(t *testing.T) {
	store := storage.New(t.TempDir())
	store.AppendAudit("CASE-AUDIT-ALIAS", "case_created", map[string]string{
		"station": "STA-ORIGINAL",
		"serial":  "SN-ORIGINAL",
	})

	snapshot := store.Audit("CASE-AUDIT-ALIAS")
	if len(snapshot) != 1 {
		t.Fatalf("TestAuditSnapshotMutationDoesNotPolluteStore: expected one audit event, got %d", len(snapshot))
	}
	snapshot[0].Data["station"] = "STA-FORGED"
	delete(snapshot[0].Data, "serial")

	stored := store.Audit("CASE-AUDIT-ALIAS")
	polluted := stored[0].Data["station"] != "STA-ORIGINAL" || stored[0].Data["serial"] != "SN-ORIGINAL"
	chainHealthy := storage.VerifyAuditChain(stored)
	if polluted || !chainHealthy {
		t.Fatalf("TestAuditSnapshotMutationDoesNotPolluteStore: polluted=%t chain_healthy=%t data=%#v", polluted, chainHealthy, stored[0].Data)
	}
}
