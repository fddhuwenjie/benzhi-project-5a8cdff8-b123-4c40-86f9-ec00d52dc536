package certificate_preview_stale_revision

import (
	"testing"
	"time"

	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
)

func TestCertificatePreviewRefreshesAfterRevisionChange(t *testing.T) {
	st := storage.New(t.TempDir())
	cases := case_service.New(st)
	reviews := review.New(cases, st)
	now := time.Date(2026, time.August, 26, 10, 0, 0, 0, time.UTC)
	c := &model.CalibrationCase{
		CaseID:                "CASE-PREVIEW-CACHE",
		StationCode:           "STA-01",
		InstrumentModel:       "SEISMO-X",
		SerialNumber:          "SN-PREVIEW-01",
		ResponsibleEngineer:   "engineer-a",
		CalibrationStandardID: "STD-01",
		Status:                model.StatusReview,
		Revision:              7,
		OpenedAt:              now,
		UpdatedAt:             now,
		Baseline:              model.Baseline{Fingerprint: "baseline-v1"},
		Batches: []model.MeasurementBatch{
			{BatchID: "B-zero", Phase: "zero", QualityState: "passed", RecordedAt: now},
			{BatchID: "B-sensitivity", Phase: "sensitivity", QualityState: "passed", RecordedAt: now.Add(time.Minute)},
			{BatchID: "B-frequency", Phase: "frequency", QualityState: "passed", RecordedAt: now.Add(2 * time.Minute)},
		},
		Reviews: []model.ReviewDecision{{
			ReviewID:       "REV-1",
			ReviewerID:     "reviewer-a",
			ReviewerRole:   "reviewer",
			Decision:       "approve",
			Rationale:      "证据完整",
			EvidenceDigest: "evidence-v1",
			SignedAt:       now.Add(3 * time.Minute),
		}},
	}
	if err := st.SaveCase(c); err != nil {
		t.Fatalf("保存测试案件失败: %v", err)
	}

	first, err := reviews.Preview(c.CaseID)
	if err != nil {
		t.Fatalf("首次生成证书预览失败: %v", err)
	}
	check, err := reviews.IssueCheck(c.CaseID)
	if err != nil {
		t.Fatalf("签发预检失败: %v", err)
	}
	issued, err := reviews.Issue(c.CaseID, c.Revision, "quality-admin", check.EvidenceDigest, first.Lock)
	if err != nil {
		t.Fatalf("签发证书失败: %v", err)
	}
	if issued.Status != model.StatusReleased {
		t.Fatalf("案件未进入released状态: %s", issued.Status)
	}

	second, err := reviews.Preview(c.CaseID)
	if err == nil {
		t.Fatalf("案件已签发后仍返回旧预览: preview_revision=%d issued_revision=%d", second.Revision, issued.Revision)
	}
}
