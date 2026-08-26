package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"seismocal/internal/model"
	"seismocal/internal/storage"
	"time"
)

// CertificateVerification is a deterministic, read-only integrity report for
// an issued case. The report deliberately contains each independent check so
// an administrator can see which part of the archive is blocking release.
type CertificateVerification struct {
	CaseID                 string    `json:"case_id"`
	Valid                  bool      `json:"valid"`
	CertificateID          string    `json:"certificate_id"`
	CertificateDigestMatch bool      `json:"certificate_digest_match"`
	AuditChainHealthy      bool      `json:"audit_chain_healthy"`
	AuditHeadMatch         bool      `json:"audit_head_match"`
	EvidenceDigestMatch    bool      `json:"evidence_digest_match"`
	RetentionState         string    `json:"retention_state"`
	RetentionDays          int       `json:"retention_days"`
	RetentionUntil         time.Time `json:"retention_until"`
	Blockers               []string  `json:"blockers"`
	CheckedAt              time.Time `json:"checked_at"`
}

// VerifyCertificate recomputes certificate, audit and evidence digests and
// evaluates the archive retention window. It never updates a case, revision,
// certificate or inspection record.
func (s *Service) VerifyCertificate(id string) (CertificateVerification, error) {
	now := time.Now().UTC()
	r := CertificateVerification{CaseID: id, CheckedAt: now, RetentionState: "已过期", Blockers: make([]string, 0)}
	if err := model.ValidateIdentifier("case_id", id); err != nil {
		return r, err
	}
	c, ok := s.store.GetCase(id)
	if !ok {
		return r, fmt.Errorf("案件不存在")
	}
	if c.Status != model.StatusReleased {
		r.Blockers = append(r.Blockers, "案件未处于released状态")
	}
	if c.Certificate == nil {
		r.Blockers = append(r.Blockers, "缺失CertificateArchive")
	} else {
		cert := c.Certificate
		r.CertificateID = cert.CertificateID
		r.RetentionUntil = cert.RetentionUntil
		r.CertificateDigestMatch = cert.CertificateDigest == CertificateDigest(*c)
		if !r.CertificateDigestMatch {
			r.Blockers = append(r.Blockers, "证书摘要不匹配")
		}
		if cert.RetentionUntil.IsZero() {
			r.Blockers = append(r.Blockers, "证书保留期限缺失")
		} else {
			d := cert.RetentionUntil.Sub(now)
			if d >= 0 {
				r.RetentionDays = int(math.Ceil(d.Hours() / 24))
			} else {
				r.RetentionDays = -int(math.Ceil(-d.Hours() / 24))
			}
			switch {
			case d < 0:
				r.RetentionState = "已过期"
				r.Blockers = append(r.Blockers, "证书已过期")
			case d <= 30*24*time.Hour:
				r.RetentionState = "三十日内到期"
			default:
				r.RetentionState = "有效"
			}
		}
	}

	events, auditReadable := s.store.AuditSnapshot(id)
	r.AuditChainHealthy = auditReadable && storage.VerifyAuditChain(events)
	if !r.AuditChainHealthy {
		r.Blockers = append(r.Blockers, "审计链完整性校验失败")
	}
	head := ""
	if len(events) > 0 {
		head = events[len(events)-1].Digest
	}
	if c.Certificate != nil {
		r.AuditHeadMatch = c.Certificate.AuditHead != "" && c.Certificate.AuditHead == head
		if !r.AuditHeadMatch {
			r.Blockers = append(r.Blockers, "证书audit_head与当前链头不一致")
		}
	}

	b, archivedDigest, exists := s.store.EvidenceSnapshot(id)
	if !exists {
		r.Blockers = append(r.Blockers, "缺失证据包")
	} else {
		sum := sha256.Sum256(b)
		actual := hex.EncodeToString(sum[:])
		r.EvidenceDigestMatch = archivedDigest != "" && archivedDigest == actual
		if !r.EvidenceDigestMatch {
			r.Blockers = append(r.Blockers, "证据内容摘要不匹配")
		}
	}

	r.Valid = c.Status == model.StatusReleased && c.Certificate != nil &&
		r.CertificateDigestMatch && r.AuditChainHealthy && r.AuditHeadMatch &&
		r.EvidenceDigestMatch && r.RetentionState != "已过期" && len(r.Blockers) == 0
	return r, nil
}
