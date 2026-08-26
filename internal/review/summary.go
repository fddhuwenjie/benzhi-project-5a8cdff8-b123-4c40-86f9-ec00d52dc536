package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"seismocal/internal/model"
	"sort"
)

func EvidenceDigest(c model.CalibrationCase) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", c.CaseID, c.SerialNumber, c.Baseline.Fingerprint, c.CalibrationStandardID)
	for _, b := range c.Batches {
		fmt.Fprintf(h, "|%s|%s|%d|%.8f|%.8f", b.Phase, b.SampleDigest, b.SampleCount, b.Mean, b.StdDev)
	}
	// 整改证据属于案件证据状态；按整改项和证据编号排序，保证摘要规范且可重算。
	remediations := append([]model.RemediationItem(nil), c.Remediations...)
	sort.SliceStable(remediations, func(i, j int) bool { return remediations[i].ItemID < remediations[j].ItemID })
	for _, r := range remediations {
		evidence := append([]string(nil), r.EvidenceBatchIDs...)
		sort.Strings(evidence)
		fmt.Fprintf(h, "|remediation|%s|%s|%s|%s|%s", r.ItemID, r.Status, r.Phase, r.AnomalyID, r.Explanation)
		for _, id := range evidence {
			fmt.Fprintf(h, "|evidence|%s", id)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func CertificateDigest(c model.CalibrationCase) string {
	h := sha256.New()
	last := ""
	if len(c.Reviews) > 0 {
		last = c.Reviews[len(c.Reviews)-1].EvidenceDigest
	}
	// Issuance persists the certificate in the same update that advances the
	// case revision. Normalize that post-issuance revision back to the signed
	// revision so previews and archived certificates retain one digest.
	revision := c.Revision
	if c.Status == model.StatusReleased && c.Certificate != nil && revision > 0 {
		revision--
	}
	fmt.Fprintf(h, "%s|%s|%s|%s|%s|%d", c.CaseID, c.SerialNumber, c.Baseline.Fingerprint, batchSummary(c), last, revision)
	return hex.EncodeToString(h.Sum(nil))
}
