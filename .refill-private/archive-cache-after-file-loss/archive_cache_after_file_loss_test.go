package archivecacheafterfileloss_test

import (
	"os"
	"path/filepath"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"testing"
)

func TestSnapshotsRejectDeletedPersistentFiles(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	caseID := "CASE-ARCHIVE-LOSS"
	c := &model.CalibrationCase{
		CaseID: caseID,
		Status: model.StatusReleased,
		Certificate: &model.CertificateArchive{
			CaseID: caseID,
		},
	}
	if err := store.SaveCase(c); err != nil {
		t.Fatalf("保存案件失败: %v", err)
	}
	event := store.AppendAudit(caseID, "certificate_issued", map[string]string{"certificate": "CERT-LOSS"})
	c.Certificate.AuditHead = event.Digest
	if err := store.SaveCase(c); err != nil {
		t.Fatalf("保存审计头失败: %v", err)
	}
	if _, err := store.Evidence(caseID); err != nil {
		t.Fatalf("生成证据包失败: %v", err)
	}

	reloaded := storage.New(dir)
	evidencePath := filepath.Join(dir, "evidence-"+caseID+".json")
	auditPath := filepath.Join(dir, caseID+".audit")
	if err := os.Remove(evidencePath); err != nil {
		t.Fatalf("删除证据文件失败: %v", err)
	}
	if err := os.Remove(auditPath); err != nil {
		t.Fatalf("删除审计文件失败: %v", err)
	}

	_, _, evidenceOK := reloaded.EvidenceSnapshot(caseID)
	_, auditOK := reloaded.AuditSnapshot(caseID)
	verification, err := review.New(case_service.New(reloaded), reloaded).VerifyCertificate(caseID)
	if err != nil {
		t.Fatalf("证书验证失败: %v", err)
	}
	missingEvidence := hasBlocker(verification.Blockers, "缺失证据包")
	brokenAudit := hasBlocker(verification.Blockers, "审计链完整性校验失败")
	if evidenceOK || auditOK || !missingEvidence || !brokenAudit {
		t.Fatalf("持久化文件已失效但验证仍回退到缓存: evidence_ok=%t audit_ok=%t missing_evidence=%t broken_audit=%t", evidenceOK, auditOK, missingEvidence, brokenAudit)
	}
}

func hasBlocker(blockers []string, want string) bool {
	for _, blocker := range blockers {
		if blocker == want {
			return true
		}
	}
	return false
}
