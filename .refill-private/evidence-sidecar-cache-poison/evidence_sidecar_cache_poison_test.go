package evidence_sidecar_cache_poison_test

import (
	"os"
	"path/filepath"
	"seismocal/internal/model"
	"seismocal/internal/storage"
	"strings"
	"testing"
	"time"
)

func TestEvidenceRetryDoesNotUseFailedArchiveCache(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	caseID := "CASE-EVIDENCE-CACHE"
	event := store.AppendAudit(caseID, "certificate_issued", map[string]string{"certificate": "CERT-CACHE"})
	c := &model.CalibrationCase{
		CaseID:                caseID,
		StationCode:           "STA-CACHE",
		InstrumentModel:       "SEIS-CACHE",
		SerialNumber:          "SN-CACHE",
		ResponsibleEngineer:   "engineer-cache",
		CalibrationStandardID: "STD-CACHE",
		Status:                model.StatusReleased,
		Revision:              9,
		OpenedAt:              time.Now().UTC(),
		Certificate: &model.CertificateArchive{
			CertificateID:     "CERT-CACHE",
			CaseID:            caseID,
			CertificateDigest: "digest-cache",
			AuditHead:         event.Digest,
		},
	}
	if err := store.SaveCase(c); err != nil {
		t.Fatalf("准备案件失败: %v", err)
	}
	sidecar := filepath.Join(dir, "evidence-"+caseID+".sha256")
	if err := os.Mkdir(sidecar, 0755); err != nil {
		t.Fatalf("准备失效侧车路径失败: %v", err)
	}
	if _, err := store.Evidence(caseID); err == nil || !strings.Contains(err.Error(), "证据摘要写入失败") {
		t.Fatalf("首次封存应报告侧车持久化错误，实际为 %v", err)
	}
	if _, err := store.Evidence(caseID); err == nil {
		t.Fatalf("重试在侧车仍失效时错误命中未完成归档缓存")
	}
}
