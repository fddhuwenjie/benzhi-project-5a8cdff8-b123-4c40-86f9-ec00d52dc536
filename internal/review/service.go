package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/storage"
	"strings"
	"sync"
	"time"
)

type Service struct {
	cases     *case_service.Service
	store     *storage.Store
	previewMu sync.RWMutex
	previews  map[string]CertificatePreview
}

type IssueCheck struct {
	Ready                bool     `json:"ready"`
	Blockers             []string `json:"blockers"`
	EvidenceDigest       string   `json:"evidence_digest,omitempty"`
	Revision             int      `json:"revision"`
	RemediationTotal     int      `json:"remediation_total"`
	RemediationCompleted int      `json:"remediation_completed"`
	EvidenceCoverage     float64  `json:"evidence_coverage"`
}

type CertificatePreview struct {
	CaseID            string `json:"case_id"`
	Revision          int    `json:"revision"`
	BaselineDigest    string `json:"baseline_digest"`
	BatchesDigest     string `json:"batches_digest"`
	ReviewDigest      string `json:"review_digest"`
	CertificateDigest string `json:"certificate_digest"`
	Lock              string `json:"lock"`
}

func (s *Service) Preview(id string) (CertificatePreview, error) {
	s.previewMu.RLock()
	if cached, ok := s.previews[id]; ok {
		s.previewMu.RUnlock()
		return cached, nil
	}
	s.previewMu.RUnlock()
	c, ok := s.store.GetCase(id)
	if !ok {
		return CertificatePreview{}, fmt.Errorf("案件不存在")
	}
	check, _ := s.IssueCheck(id)
	if !check.Ready {
		return CertificatePreview{}, fmt.Errorf("签发预检未通过: %s", strings.Join(check.Blockers, ";"))
	}
	bd := c.Baseline.Fingerprint
	bs := batchSummary(*c)
	rd := ""
	if len(c.Reviews) > 0 {
		rd = c.Reviews[len(c.Reviews)-1].EvidenceDigest
	}
	cd := CertificateDigest(*c)
	lock := fmt.Sprintf("%d|%s|%s|%s|%s", c.Revision, bd, bs, rd, cd)
	preview := CertificatePreview{CaseID: id, Revision: c.Revision, BaselineDigest: bd, BatchesDigest: bs, ReviewDigest: rd, CertificateDigest: cd, Lock: lock}
	s.previewMu.Lock()
	s.previews[id] = preview
	s.previewMu.Unlock()
	return preview, nil
}

func (s *Service) IssueCheck(id string) (IssueCheck, error) {
	c, ok := s.store.GetCase(id)
	if !ok {
		return IssueCheck{}, fmt.Errorf("案件不存在")
	}
	out := IssueCheck{Revision: c.Revision}
	// Always expose the current digest so a blocked case can still be corrected
	// and submitted for independent re-review with the latest evidence state.
	out.EvidenceDigest = EvidenceDigest(*c)
	if c.Status != model.StatusReview {
		out.Blockers = append(out.Blockers, "案件未处于review状态")
	}
	if !calibration.MeasurementComplete(*c) {
		out.Blockers = append(out.Blockers, "三阶段测量未完备")
	}
	if !calibration.AllAnomaliesResolved(*c) {
		out.Blockers = append(out.Blockers, "存在开放异常")
	}
	if c.EnvironmentTrend.Blocked && !c.EnvironmentTrend.Resolved {
		out.Blockers = append(out.Blockers, "存在未处置环境漂移趋势")
	}
	out.RemediationTotal = len(c.Remediations)
	for _, r := range c.Remediations {
		if r.Status == "completed" {
			out.RemediationCompleted++
			if len(r.EvidenceBatchIDs) > 0 {
				out.EvidenceCoverage += 1
			}
		}
	}
	if out.RemediationTotal > 0 {
		out.EvidenceCoverage /= float64(out.RemediationTotal)
	}
	if len(c.Reviews) == 0 || !strings.EqualFold(c.Reviews[len(c.Reviews)-1].Decision, "approve") {
		out.Blockers = append(out.Blockers, "复核未通过")
	}
	for _, r := range c.Remediations {
		if r.Status != "completed" {
			out.Blockers = append(out.Blockers, "未完成整改: "+r.ItemID)
		}
	}
	if len(out.Blockers) == 0 {
		out.Ready = true
	}
	return out, nil
}

func New(c *case_service.Service, s *storage.Store) *Service {
	return &Service{cases: c, store: s, previews: map[string]CertificatePreview{}}
}

var independentRoles = map[string]bool{"reviewer": true, "independent_reviewer": true, "quality_reviewer": true, "复核专家": true}

func (s *Service) Submit(id string, rev int, r model.ReviewDecision) (*model.CalibrationCase, error) {
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	if pending := pendingRemediations(*c); len(pending) > 0 {
		return nil, fmt.Errorf("存在未完成整改项: %s", strings.Join(pending, ", "))
	}
	if !EligibleForReview(*c) {
		return nil, fmt.Errorf("案件不满足复核条件")
	}
	if r.ReviewerID == "" || r.ReviewerID == c.ResponsibleEngineer {
		return nil, fmt.Errorf("复核人必须独立")
	}
	if !independentRoles[strings.ToLower(strings.TrimSpace(r.ReviewerRole))] {
		return nil, fmt.Errorf("复核角色不在白名单")
	}
	if strings.TrimSpace(r.Rationale) == "" {
		return nil, fmt.Errorf("rationale不能为空")
	}
	if !strings.EqualFold(r.Decision, "approve") && !strings.EqualFold(r.Decision, "reject") {
		return nil, fmt.Errorf("decision无效")
	}
	if strings.EqualFold(r.Decision, "reject") && (strings.TrimSpace(r.RejectCategory) == "" || (strings.TrimSpace(r.AffectedPhase) == "" && strings.TrimSpace(r.AffectedAnomaly) == "")) {
		return nil, fmt.Errorf("驳回必须填写类别及受影响阶段或异常")
	}
	currentDigest := EvidenceDigest(*c)
	if r.EvidenceDigest == "" {
		return nil, fmt.Errorf("证据摘要不能为空，当前摘要为 %s", currentDigest)
	}
	if r.EvidenceDigest != currentDigest {
		return nil, fmt.Errorf("证据摘要不一致: 当前案件摘要为 %s", currentDigest)
	}
	r.ReviewID = fmt.Sprintf("REV-%x", sha256.Sum256([]byte(id+"|"+r.ReviewerID+"|"+r.EvidenceDigest)))
	r.CaseID = id
	r.SignedAt = time.Now().UTC()
	out, err := s.cases.Update(id, func(c *model.CalibrationCase) error {
		c.Reviews = append(c.Reviews, r)
		if strings.EqualFold(r.Decision, "approve") {
			c.Status = model.StatusReview
		} else {
			for _, old := range c.Remediations {
				if old.Status != "completed" {
					old.Status = "open"
				}
			}
			cat := r.RejectCategory
			if cat == "" {
				cat = "review_reject"
			}
			c.Remediations = append(c.Remediations, model.RemediationItem{ItemID: "REM-" + r.ReviewID, Category: cat, Reason: r.Rationale, Phase: r.AffectedPhase, AnomalyID: r.AffectedAnomaly, Status: "open"})
			c.Status = model.StatusMeasuring
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.store.AppendAudit(id, "review_submitted", map[string]string{"reviewer": r.ReviewerID, "role": r.ReviewerRole, "decision": r.Decision, "evidence_digest": r.EvidenceDigest})
	return out, nil
}

func (s *Service) UpdateRemediation(id string, rev int, itemID, explanation string, evidence []string) (*model.CalibrationCase, error) {
	if strings.TrimSpace(explanation) == "" || len(evidence) == 0 {
		return nil, fmt.Errorf("整改说明和证据不能为空")
	}
	out, err := s.cases.Update(id, func(c *model.CalibrationCase) error {
		if c.Revision != rev {
			return fmt.Errorf("revision冲突")
		}
		for i := range c.Remediations {
			if c.Remediations[i].ItemID == itemID {
				if c.Remediations[i].Status == "completed" {
					return fmt.Errorf("整改项已完成")
				}
				if strings.TrimSpace(c.Remediations[i].Explanation) != "" || len(c.Remediations[i].EvidenceBatchIDs) > 0 {
					return fmt.Errorf("整改项已有提交记录，不可覆盖")
				}
				seen := map[string]bool{}
				for _, id := range evidence {
					if seen[id] {
						return fmt.Errorf("证据批次重复引用: %s", id)
					}
					seen[id] = true
					found := false
					for _, b := range c.Batches {
						if b.BatchID == id {
							if b.QualityState != "passed" {
								return fmt.Errorf("证据批次仍失败: %s", id)
							}
							if c.Remediations[i].Phase != "" && normalizePhaseForReview(b.Phase) != normalizePhaseForReview(c.Remediations[i].Phase) {
								continue
							}
							found = true
						}
					}
					if !found {
						if c.Remediations[i].Phase != "" {
							return fmt.Errorf("证据覆盖不足: 需要阶段 %s", c.Remediations[i].Phase)
						}
						return fmt.Errorf("证据批次不存在: %s", id)
					}
				}
				if c.Remediations[i].AnomalyID != "" {
					covered := false
					for _, a := range c.Anomalies {
						if a.AnomalyID == c.Remediations[i].AnomalyID {
							for _, eid := range evidence {
								if eid == a.BatchID {
									covered = true
								}
								for _, at := range a.Attempts {
									if eid == at.BatchID && at.QualityCode == "PASS" {
										covered = true
									}
								}
							}
						}
					}
					if !covered {
						return fmt.Errorf("证据未覆盖异常: %s", c.Remediations[i].AnomalyID)
					}
				}
				c.Remediations[i].Explanation = explanation
				c.Remediations[i].EvidenceBatchIDs = evidence
				c.Remediations[i].Status = "completed"
				c.Remediations[i].CompletedAt = time.Now().UTC()
				return nil
			}
		}
		return fmt.Errorf("整改项不存在")
	})
	if err != nil {
		return nil, err
	}
	digest := EvidenceDigest(*out)
	ids := strings.Join(evidence, ",")
	s.store.AppendAudit(id, "remediation_completed", map[string]string{
		"item_id":            itemID,
		"explanation":        explanation,
		"evidence_batch_ids": ids,
		"evidence_digest":    digest,
	})
	return out, nil
}

func pendingRemediations(c model.CalibrationCase) []string {
	ids := make([]string, 0)
	for _, r := range c.Remediations {
		if r.Status != "completed" {
			ids = append(ids, r.ItemID)
		}
	}
	return ids
}

func normalizePhaseForReview(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "zero", "sensitivity", "frequency":
		return strings.ToLower(strings.TrimSpace(p))
	}
	return strings.ToLower(strings.TrimSpace(p))
}
func (s *Service) Issue(id string, rev int, issuer string, expected ...string) (*model.CalibrationCase, error) {
	if strings.TrimSpace(issuer) == "" {
		return nil, fmt.Errorf("issued_by不能为空")
	}
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	if c.Status != model.StatusReview {
		return nil, fmt.Errorf("案件不在review状态")
	}
	if check, _ := s.IssueCheck(id); !check.Ready {
		return nil, fmt.Errorf("签发预检未通过: %s", strings.Join(check.Blockers, ";"))
	}
	if len(expected) > 0 && expected[0] != "" {
		if check, _ := s.IssueCheck(id); check.EvidenceDigest != expected[0] {
			return nil, fmt.Errorf("签发摘要不一致")
		}
	}
	if len(expected) > 1 && expected[1] != "" {
		p, e := s.Preview(id)
		if e != nil || p.Lock != expected[1] {
			return nil, fmt.Errorf("预览已过期")
		}
	}
	if !calibration.AllAnomaliesResolved(*c) {
		return nil, fmt.Errorf("存在未解决异常")
	}
	if len(c.Batches) == 0 {
		return nil, fmt.Errorf("没有测量批次")
	}
	for _, b := range c.Batches {
		if !BatchAccepted(*c, b) {
			return nil, fmt.Errorf("存在不合格批次")
		}
	}
	if len(c.Reviews) == 0 || strings.EqualFold(c.Reviews[len(c.Reviews)-1].Decision, "approve") == false {
		return nil, fmt.Errorf("复核未通过")
	}
	// Validate and archive the deterministic preview digest. The digest helper
	// normalizes the revision bump performed while sealing the certificate.
	digest := CertificateDigest(*c)
	issuedAt := time.Now().UTC()
	cert := &model.CertificateArchive{CertificateID: "CERT-" + digest[:20], CaseID: id, CertificateDigest: digest, IssuedBy: issuer, IssuedAt: issuedAt, RetentionUntil: issuedAt.AddDate(10, 0, 0), EvidenceBundlePath: "evidence/" + id + "-" + digest[:20] + ".json"}
	out, err := s.cases.Update(id, func(c *model.CalibrationCase) error {
		c.Certificate = cert
		c.Status = model.StatusReleased
		c.SealedAt = cert.IssuedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Persist the normalized digest after Update has attached the certificate.
	out.Certificate.CertificateDigest = CertificateDigest(*out)
	ev := s.store.AppendAudit(id, "certificate_issued", map[string]string{"certificate": cert.CertificateID, "digest": digest})
	out.Certificate.AuditHead = ev.Digest
	_ = s.store.SaveCase(out)
	if _, e := s.store.Evidence(id); e != nil {
		return nil, fmt.Errorf("证据包固化失败: %w", e)
	}
	return out, nil
}
func batchSummary(c model.CalibrationCase) string {
	h := sha256.New()
	for _, b := range c.Batches {
		fmt.Fprintf(h, "%s|%s|%d|%.8f|%.8f", b.Phase, b.SampleDigest, b.SampleCount, b.Mean, b.StdDev)
	}
	return hex.EncodeToString(h.Sum(nil))
}
