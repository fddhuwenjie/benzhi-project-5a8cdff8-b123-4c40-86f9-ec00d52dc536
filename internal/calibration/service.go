package calibration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/storage"
	"strings"
	"time"
)

type Service struct {
	cases *case_service.Service
	store *storage.Store
}

func New(c *case_service.Service, s *storage.Store) *Service { return &Service{cases: c, store: s} }

var phaseOrder = []string{"zero", "sensitivity", "frequency"}

func normalizePhase(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "零点", "zero", "zeroing", "phase_zero":
		return "zero"
	case "灵敏度", "sensitivity", "sensitivity_check", "phase_sensitivity":
		return "sensitivity"
	case "频响", "frequency", "frequency_response", "phase_frequency":
		return "frequency"
	}
	return ""
}
func digest(v []float64) string {
	h := sha256.New()
	for _, x := range v {
		fmt.Fprintf(h, "%.8f,", x)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func batchID(id, phase, d string) string {
	h := sha256.Sum256([]byte(id + "|" + phase + "|" + d))
	return "B-" + hex.EncodeToString(h[:])[:16]
}
func assess(c *model.CalibrationCase, b model.MeasurementBatch) (QualityReport, error) {
	if b.SampleCount <= 0 || b.SampleCount != len(b.Values) {
		return QualityReport{}, fmt.Errorf("样本完整性失败")
	}
	for _, v := range b.Values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return QualityReport{}, fmt.Errorf("样本包含非有限数值")
		}
	}
	phase := normalizePhase(b.Phase)
	if phase == "" {
		return QualityReport{}, fmt.Errorf("阶段无效")
	}
	idx := 0
	for _, old := range c.Batches {
		if !old.Retest {
			idx++
		}
	}
	if idx >= len(phaseOrder) || phaseOrder[idx] != phase {
		return QualityReport{}, fmt.Errorf("阶段顺序错误: expected=%s", phaseOrder[min(idx, len(phaseOrder)-1)])
	}
	for _, old := range c.Batches {
		if !old.Retest && normalizePhase(old.Phase) == phase {
			return QualityReport{}, fmt.Errorf("阶段已提交")
		}
	}
	r := Evaluate(b.Values)
	if mathAbs(b.TemperatureC-c.Baseline.TemperatureC) > 5 || mathAbs(b.HumidityPercent-c.Baseline.HumidityPercent) > 10 {
		r.Code = "ENVIRONMENT_DRIFT"
		r.EnvironmentOK = false
	}
	return r, nil
}

// EnvironmentTrend computes cross-batch drift relative to the frozen baseline.
func EnvironmentTrend(c model.CalibrationCase) model.EnvironmentTrend {
	out := model.EnvironmentTrend{Phases: map[string]model.EnvironmentPhaseTrend{}}
	var n float64
	consecutive := 0
	previous := ""
	previousSign := 0
	for _, b := range c.Batches {
		if b.Retest {
			continue
		}
		td := mathAbs(b.TemperatureC - c.Baseline.TemperatureC)
		hd := mathAbs(b.HumidityPercent - c.Baseline.HumidityPercent)
		p := normalizePhase(b.Phase)
		ps := out.Phases[p]
		ps.BatchCount++
		if td > ps.MaxTemperatureDeviation {
			ps.MaxTemperatureDeviation = td
		}
		if hd > ps.MaxHumidityDeviation {
			ps.MaxHumidityDeviation = hd
		}
		ps.AverageTemperatureDeviation += td
		ps.AverageHumidityDeviation += hd
		out.Phases[p] = ps
		if td > out.MaxTemperatureDeviation {
			out.MaxTemperatureDeviation = td
		}
		if hd > out.MaxHumidityDeviation {
			out.MaxHumidityDeviation = hd
		}
		out.AverageTemperatureDeviation += td
		out.AverageHumidityDeviation += hd
		n++
		// A trend point is a same-direction, non-trivial drift. Two adjacent points gate the case.
		sign := 0
		if mathAbs(b.TemperatureC-c.Baseline.TemperatureC) >= 2 {
			if b.TemperatureC >= c.Baseline.TemperatureC {
				sign = 1
			} else {
				sign = -1
			}
		} else if mathAbs(b.HumidityPercent-c.Baseline.HumidityPercent) >= 5 {
			if b.HumidityPercent >= c.Baseline.HumidityPercent {
				sign = 1
			} else {
				sign = -1
			}
		}
		if (td >= 2 || hd >= 5) && (previousSign == 0 || sign == previousSign) {
			consecutive++
		} else {
			consecutive = 0
		}
		if consecutive >= 2 {
			out.ConsecutiveDriftCount = consecutive
			out.Blocked = true
			if len(out.TriggerBatchIDs) == 0 && previous != "" {
				out.TriggerBatchIDs = append(out.TriggerBatchIDs, previous)
			}
			if len(out.TriggerBatchIDs) == 0 || out.TriggerBatchIDs[len(out.TriggerBatchIDs)-1] != b.BatchID {
				out.TriggerBatchIDs = append(out.TriggerBatchIDs, b.BatchID)
			}
		}
		if td >= 8 || hd >= 20 {
			out.Blocked = true
			if len(out.TriggerBatchIDs) == 0 || out.TriggerBatchIDs[len(out.TriggerBatchIDs)-1] != b.BatchID {
				out.TriggerBatchIDs = append(out.TriggerBatchIDs, b.BatchID)
			}
		}
		previous = b.BatchID
		if sign != 0 {
			previousSign = sign
		}
	}
	if n > 0 {
		out.AverageTemperatureDeviation /= n
		out.AverageHumidityDeviation /= n
	}
	for p, ps := range out.Phases {
		if ps.BatchCount > 0 {
			ps.AverageTemperatureDeviation /= float64(ps.BatchCount)
			ps.AverageHumidityDeviation /= float64(ps.BatchCount)
		}
		out.Phases[p] = ps
	}
	return out
}
func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
func (s *Service) AddBatch(id string, rev int, b model.MeasurementBatch) (*model.CalibrationCase, error) {
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	if c.Status != model.StatusMeasuring {
		return nil, fmt.Errorf("当前状态不可测量")
	}
	r, e := assess(c, b)
	if e != nil {
		return nil, e
	}
	b.CaseID = id
	b.Phase = normalizePhase(b.Phase)
	b.RecordedAt = time.Now().UTC()
	b.SampleDigest = digest(b.Values)
	b.BatchID = batchID(id, b.Phase, b.SampleDigest)
	b.TemperatureDeviation = mathAbs(b.TemperatureC - c.Baseline.TemperatureC)
	b.HumidityDeviation = mathAbs(b.HumidityPercent - c.Baseline.HumidityPercent)
	b.QualityState = "passed"
	if r.Code != "PASS" {
		b.QualityState = "failed"
		b.AnomalyCode = r.Code
	}
	b.Mean = bMean(b.Values)
	b.StdDev = r.StdDev
	b.Quality = r.Metrics()
	out, e := s.cases.Update(id, func(c *model.CalibrationCase) error {
		if c.Revision != rev {
			return fmt.Errorf("revision冲突")
		}
		if c.Status != model.StatusMeasuring {
			return fmt.Errorf("当前状态不可测量")
		}
		c.Batches = append(c.Batches, b)
		trend := EnvironmentTrend(*c)
		c.EnvironmentTrend = trend
		if trend.Blocked {
			found := false
			for _, a := range c.Anomalies {
				if !a.Resolved && a.Code == "ENVIRONMENT_TREND" {
					found = true
				}
			}
			if !found {
				bid := b.BatchID
				if len(trend.TriggerBatchIDs) > 0 {
					bid = trend.TriggerBatchIDs[len(trend.TriggerBatchIDs)-1]
				}
				c.Anomalies = append(c.Anomalies, model.Anomaly{AnomalyID: "AN-TREND-" + bid, Code: "ENVIRONMENT_TREND", BatchID: bid, Description: "跨批次环境持续漂移", CreatedAt: time.Now().UTC()})
			}
			c.Status = model.StatusPaused
		}
		if b.QualityState == "failed" {
			c.Anomalies = append(c.Anomalies, model.Anomaly{AnomalyID: "AN-" + b.BatchID, Code: b.AnomalyCode, BatchID: b.BatchID, Description: "质量门禁失败", CreatedAt: time.Now().UTC()})
			c.Status = model.StatusPaused
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	s.store.AppendAudit(id, "measurement_recorded", map[string]string{"batch_id": b.BatchID, "phase": b.Phase, "quality": b.QualityState, "code": b.AnomalyCode})
	if b.QualityState == "failed" {
		s.store.AppendAudit(id, "anomaly_created", map[string]string{"batch_id": b.BatchID, "code": b.AnomalyCode})
	}
	if out.EnvironmentTrend.Blocked {
		s.store.AppendAudit(id, "environment_trend_anomaly", map[string]string{"trigger_batches": strings.Join(out.EnvironmentTrend.TriggerBatchIDs, ",")})
	}
	return out, nil
}

func CompletedPhases(c model.CalibrationCase) map[string]bool {
	out := map[string]bool{}
	for _, b := range c.Batches {
		if b.QualityState == "passed" {
			out[normalizePhase(b.Phase)] = true
		}
	}
	return out
}
func MeasurementComplete(c model.CalibrationCase) bool {
	p := CompletedPhases(c)
	return p["zero"] && p["sensitivity"] && p["frequency"] && len(OpenAnomalies(c)) == 0
}

func (s *Service) AddBatches(id string, rev int, batches []model.MeasurementBatch) (*model.CalibrationCase, []map[string]any, error) {
	if len(batches) == 0 || len(batches) > 20 {
		return nil, nil, fmt.Errorf("batches数量超限")
	}
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, nil, fmt.Errorf("revision冲突")
	}
	if c.Status != model.StatusMeasuring {
		return nil, nil, fmt.Errorf("当前状态不可测量")
	}
	// 预检整组，保证任一失败时不写入
	working := *c
	working.Batches = append([]model.MeasurementBatch(nil), c.Batches...)
	prepared := make([]model.MeasurementBatch, 0, len(batches))
	results := make([]map[string]any, 0, len(batches))
	for i, b := range batches {
		if b.Retest {
			return nil, []map[string]any{{"index": i, "error": "批量导入不允许复测批次"}}, fmt.Errorf("批量导入不允许复测批次")
		}
		if b.SampleDigest != "" && b.SampleDigest != digest(b.Values) {
			return nil, []map[string]any{{"index": i, "error": "sample_digest不匹配"}}, fmt.Errorf("sample_digest不匹配")
		}
		r, e := assess(&working, b)
		if e != nil {
			return nil, []map[string]any{{"index": i, "error": e.Error()}}, e
		}
		b.CaseID = id
		b.Phase = normalizePhase(b.Phase)
		b.RecordedAt = time.Now().UTC()
		b.SampleDigest = digest(b.Values)
		b.BatchID = batchID(id, b.Phase, b.SampleDigest)
		b.TemperatureDeviation = mathAbs(b.TemperatureC - c.Baseline.TemperatureC)
		b.HumidityDeviation = mathAbs(b.HumidityPercent - c.Baseline.HumidityPercent)
		b.QualityState = "passed"
		if r.Code != "PASS" {
			b.QualityState = "failed"
			b.AnomalyCode = r.Code
		}
		b.Mean = bMean(b.Values)
		b.StdDev = r.StdDev
		b.Quality = r.Metrics()
		prepared = append(prepared, b)
		working.Batches = append(working.Batches, b)
		results = append(results, map[string]any{"index": i, "batch_id": b.BatchID, "status": map[bool]string{true: "passed", false: "failed"}[b.QualityState == "passed"]})
		if b.QualityState == "failed" {
			for j := i + 1; j < len(batches); j++ {
				results = append(results, map[string]any{"index": j, "status": "not_executed"})
			}
			break
		}
	}
	out, e := s.cases.Update(id, func(c *model.CalibrationCase) error {
		if c.Revision != rev {
			return fmt.Errorf("revision冲突")
		}
		for _, b := range prepared {
			c.Batches = append(c.Batches, b)
			c.EnvironmentTrend = EnvironmentTrend(*c)
			if c.EnvironmentTrend.Blocked {
				c.Status = model.StatusPaused
			}
			if b.QualityState == "failed" {
				c.Anomalies = append(c.Anomalies, model.Anomaly{AnomalyID: "AN-" + b.BatchID, Code: b.AnomalyCode, BatchID: b.BatchID, Description: "质量门禁失败", CreatedAt: time.Now().UTC()})
				c.Status = model.StatusPaused
			}
		}
		return nil
	})
	if e != nil {
		return nil, nil, e
	}
	for _, b := range prepared {
		s.store.AppendAudit(id, "measurement_recorded", map[string]string{"batch_id": b.BatchID, "phase": b.Phase, "quality": b.QualityState})
		if b.QualityState == "failed" {
			s.store.AppendAudit(id, "anomaly_created", map[string]string{"batch_id": b.BatchID, "code": b.AnomalyCode})
		}
	}
	if out.EnvironmentTrend.Blocked {
		s.store.AppendAudit(id, "environment_trend_anomaly", map[string]string{"trigger_batches": strings.Join(out.EnvironmentTrend.TriggerBatchIDs, ",")})
	}
	return out, results, nil
}
func bMean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
func (s *Service) ResolveAnomaly(id string, rev int, b model.MeasurementBatch, disp string) (*model.CalibrationCase, error) {
	if strings.TrimSpace(disp) == "" {
		return nil, fmt.Errorf("disposition不能为空")
	}
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	if c.Status != model.StatusPaused {
		return nil, fmt.Errorf("仅暂停案件可复测")
	}
	var target *model.Anomaly
	for i := range c.Anomalies {
		if !c.Anomalies[i].Resolved {
			if target == nil {
				target = &c.Anomalies[i]
			}
			if b.BatchID != "" && c.Anomalies[i].BatchID == b.BatchID {
				target = &c.Anomalies[i]
				break
			}
		}
	}
	if target == nil {
		return nil, fmt.Errorf("没有未解决异常")
	}
	if len(target.Attempts) >= 3 {
		return nil, fmt.Errorf("异常复测次数超限: %s", target.AnomalyID)
	}
	if b.SampleCount <= 0 {
		return nil, fmt.Errorf("sample_count无效")
	}
	if b.SampleCount != len(b.Values) || len(b.Values) == 0 {
		return nil, fmt.Errorf("样本完整性失败")
	}
	r := Evaluate(b.Values)
	if mathAbs(b.TemperatureC-c.Baseline.TemperatureC) > 5 || mathAbs(b.HumidityPercent-c.Baseline.HumidityPercent) > 10 {
		r.Code = "ENVIRONMENT_DRIFT"
	}
	b.CaseID = id
	b.Phase = normalizePhase(b.Phase)
	if b.Phase == "" {
		return nil, fmt.Errorf("阶段无效")
	}
	originalPhase := ""
	for _, old := range c.Batches {
		if old.BatchID == target.BatchID {
			originalPhase = normalizePhase(old.Phase)
			break
		}
	}
	if originalPhase != "" && b.Phase != originalPhase {
		return nil, fmt.Errorf("复测阶段与异常批次不一致")
	}
	b.RecordedAt = time.Now().UTC()
	b.SampleDigest = digest(b.Values)
	b.BatchID = batchID(id, "retest-"+b.Phase, b.SampleDigest)
	b.TemperatureDeviation = mathAbs(b.TemperatureC - c.Baseline.TemperatureC)
	b.HumidityDeviation = mathAbs(b.HumidityPercent - c.Baseline.HumidityPercent)
	b.Retest = true
	b.QualityState = "passed"
	if r.Code != "PASS" {
		b.QualityState = "failed"
		b.AnomalyCode = r.Code
	}
	b.Mean = bMean(b.Values)
	b.StdDev = r.StdDev
	b.Quality = r.Metrics()
	out, e := s.cases.Update(id, func(c *model.CalibrationCase) error {
		if c.Revision != rev {
			return fmt.Errorf("revision冲突")
		}
		if c.Status != model.StatusPaused {
			return fmt.Errorf("仅暂停案件可复测")
		}
		c.Batches = append(c.Batches, b)
		for i := range c.Anomalies {
			if c.Anomalies[i].BatchID == target.BatchID && !c.Anomalies[i].Resolved && b.QualityState == "passed" {
				c.Anomalies[i].Resolved = true
				c.Anomalies[i].Disposition = disp
				c.Anomalies[i].ResolvedAt = time.Now().UTC()
			}
			if c.Anomalies[i].BatchID == target.BatchID {
				c.Anomalies[i].Attempts = append(c.Anomalies[i].Attempts, model.RetestAttempt{AttemptNo: len(c.Anomalies[i].Attempts) + 1, BatchID: b.BatchID, QualityCode: b.Quality.Code, Quality: b.Quality, Disposition: disp, Details: disp, At: b.RecordedAt})
				if b.QualityState == "failed" && len(c.Anomalies[i].Attempts) >= 3 {
					found := false
					for _, r := range c.Remediations {
						if r.AnomalyID == c.Anomalies[i].AnomalyID && r.Status != "completed" {
							found = true
						}
					}
					if !found {
						c.Remediations = append(c.Remediations, model.RemediationItem{ItemID: "ESC-" + c.Anomalies[i].AnomalyID, Category: "retest_escalation", Reason: "复测连续失败三次", AnomalyID: c.Anomalies[i].AnomalyID, Status: "open", Escalated: true})
					}
				}
			}
		}
		if b.QualityState == "passed" {
			open := false
			for _, a := range c.Anomalies {
				if !a.Resolved {
					open = true
				}
			}
			if !open {
				c.Status = model.StatusMeasuring
			}
		}
		c.EnvironmentTrend = EnvironmentTrend(*c)
		if target.Code == "ENVIRONMENT_TREND" && b.QualityState == "passed" && mathAbs(b.TemperatureC-c.Baseline.TemperatureC) < 2 && mathAbs(b.HumidityPercent-c.Baseline.HumidityPercent) < 5 {
			c.EnvironmentTrend.Blocked = false
			c.EnvironmentTrend.Resolved = true
			for i := range c.Anomalies {
				if c.Anomalies[i].Code == "ENVIRONMENT_TREND" {
					c.Anomalies[i].Resolved = true
					c.Anomalies[i].ResolvedAt = time.Now().UTC()
					c.Anomalies[i].Disposition = disp
				}
			}
		}
		if c.EnvironmentTrend.Blocked {
			c.Status = model.StatusPaused
		} else {
			for i := range c.Anomalies {
				if c.Anomalies[i].Code == "ENVIRONMENT_TREND" && !c.Anomalies[i].Resolved {
					c.Anomalies[i].Resolved = true
					c.Anomalies[i].ResolvedAt = time.Now().UTC()
					c.Anomalies[i].Disposition = disp
				}
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	s.store.AppendAudit(id, "measurement_recorded", map[string]string{"batch_id": b.BatchID, "phase": b.Phase, "quality": b.QualityState, "retest": "true"})
	if b.QualityState == "passed" {
		s.store.AppendAudit(id, "anomaly_resolved", map[string]string{"batch_id": target.BatchID, "disposition": disp})
	}
	return out, nil
}
