package case_service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"seismocal/internal/model"
	"seismocal/internal/storage"
	"sort"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store *storage.Store
	mu    sync.Mutex
}

func New(st *storage.Store) *Service { return &Service{store: st} }
func (s *Service) Create(id, station, modelName, serial, engineer, standard string) (*model.CalibrationCase, error) {
	now := time.Now().UTC()
	c := &model.CalibrationCase{CaseID: strings.TrimSpace(id), StationCode: strings.TrimSpace(station), InstrumentModel: strings.TrimSpace(modelName), SerialNumber: strings.TrimSpace(serial), ResponsibleEngineer: strings.TrimSpace(engineer), CalibrationStandardID: strings.TrimSpace(standard), Status: model.StatusDraft, Revision: 1, OpenedAt: now, UpdatedAt: now}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.store.GetCase(c.CaseID); ok {
		return nil, fmt.Errorf("case_id已存在: %s", old.CaseID)
	}
	if old, ok := s.store.FindActiveByInstrument(c.StationCode, c.SerialNumber); ok {
		return nil, fmt.Errorf("仪器档案冲突: existing_case_id=%s", old.CaseID)
	}
	if err := s.store.SaveCase(c); err != nil {
		return nil, err
	}
	s.store.AppendAudit(c.CaseID, "case_created", map[string]string{"station": c.StationCode, "serial": c.SerialNumber})
	return c, nil
}
func baselineFingerprint(b model.Baseline, standard string) string {
	gauge := canonicalGauge(b.GaugeConfig)
	raw, _ := json.Marshal(struct {
		T, H            float64
		Gauge, Standard string
	}{b.TemperatureC, b.HumidityPercent, gauge, standard})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func canonicalGauge(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return strings.TrimSpace(raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func ValidateGaugeConfig(raw string, opened time.Time) (string, []string, error) {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return "", nil, fmt.Errorf("gauge_config格式无效")
	}
	arr, ok := v.([]any)
	if !ok {
		return canonicalGauge(raw), nil, nil
	}
	type gauge struct {
		ID          string `json:"gauge_id"`
		Type        string `json:"type"`
		Certificate string `json:"traceability_certificate"`
		ValidUntil  string `json:"valid_until"`
		Purpose     string `json:"purpose"`
	}
	seen := map[string]bool{}
	warns := []string{}
	norm := make([]gauge, 0, len(arr))
	for i, x := range arr {
		b, _ := json.Marshal(x)
		var g gauge
		if json.Unmarshal(b, &g) != nil || strings.TrimSpace(g.ID) == "" || strings.TrimSpace(g.Type) == "" || strings.TrimSpace(g.Certificate) == "" || strings.TrimSpace(g.ValidUntil) == "" || strings.TrimSpace(g.Purpose) == "" {
			return "", nil, fmt.Errorf("gauge_config[%d]字段不完整", i)
		}
		if seen[g.ID] {
			return "", nil, fmt.Errorf("量具编号重复: %s", g.ID)
		}
		seen[g.ID] = true
		t, e := time.Parse(time.RFC3339, g.ValidUntil)
		if e != nil {
			t, e = time.Parse("2006-01-02", g.ValidUntil)
		}
		if e != nil {
			return "", nil, fmt.Errorf("gauge_config[%d].valid_until无效", i)
		}
		if t.Before(opened) {
			return "", nil, fmt.Errorf("量具已过期: %s", g.ID)
		}
		if t.Sub(opened) <= 30*24*time.Hour {
			warns = append(warns, "量具临近到期: "+g.ID)
		}
		norm = append(norm, g)
	}
	sort.Slice(norm, func(i, j int) bool { return norm[i].ID < norm[j].ID })
	out, _ := json.Marshal(norm)
	return string(out), warns, nil
}
func (s *Service) Freeze(id string, rev int, b model.Baseline) (*model.CalibrationCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	if c.Status != model.StatusDraft {
		return nil, fmt.Errorf("当前状态不可冻结")
	}
	if b.TemperatureC < -40 || b.TemperatureC > 85 {
		return nil, fmt.Errorf("temperature_c超出范围")
	}
	if b.HumidityPercent < 0 || b.HumidityPercent > 100 {
		return nil, fmt.Errorf("humidity_percent超出范围")
	}
	if strings.TrimSpace(b.GaugeConfig) == "" {
		return nil, fmt.Errorf("gauge_config不能为空")
	}
	canon, warns, e := ValidateGaugeConfig(b.GaugeConfig, c.OpenedAt)
	if e != nil {
		return nil, e
	}
	b.GaugeConfig = canon
	b.GaugeWarnings = warns
	var parsed any
	if json.Unmarshal([]byte(b.GaugeConfig), &parsed) != nil {
		if err := model.ValidateIdentifier("gauge_config", b.GaugeConfig); err != nil {
			return nil, fmt.Errorf("gauge_config格式无效")
		}
	} else if parsed == nil {
		return nil, fmt.Errorf("gauge_config格式无效")
	}
	b.FrozenAt = time.Now().UTC()
	b.Fingerprint = baselineFingerprint(b, c.CalibrationStandardID)
	c.Baseline = b
	c.Status = model.StatusMeasuring
	c.Revision++
	c.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveCase(c); err != nil {
		return nil, err
	}
	s.store.AppendAudit(id, "baseline_frozen", map[string]string{"fingerprint": b.Fingerprint})
	return c, nil
}
func (s *Service) Transition(id string, rev int, to model.CaseStatus) (*model.CalibrationCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if c.Revision != rev {
		return nil, fmt.Errorf("revision冲突")
	}
	valid := map[model.CaseStatus][]model.CaseStatus{model.StatusMeasuring: {model.StatusPaused, model.StatusReview}, model.StatusPaused: {model.StatusMeasuring}, model.StatusReview: {model.StatusReleased}}
	allowed := false
	for _, x := range valid[c.Status] {
		if x == to {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("非法状态转换")
	}
	if to == model.StatusMeasuring {
		for _, a := range c.Anomalies {
			if !a.Resolved {
				return nil, fmt.Errorf("存在未解决异常")
			}
		}
	}
	c.Status = to
	c.Revision++
	c.UpdatedAt = time.Now().UTC()
	if to == model.StatusReleased {
		c.SealedAt = time.Now().UTC()
	}
	if err := s.store.SaveCase(c); err != nil {
		return nil, err
	}
	s.store.AppendAudit(id, "status_"+string(to), nil)
	return c, nil
}
func (s *Service) Update(id string, fn func(*model.CalibrationCase) error) (*model.CalibrationCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.store.GetCase(id)
	if !ok {
		return nil, fmt.Errorf("案件不存在")
	}
	if err := fn(c); err != nil {
		return nil, err
	}
	c.Revision++
	c.UpdatedAt = time.Now().UTC()
	if err := s.store.SaveCase(c); err != nil {
		return nil, err
	}
	return c, nil
}
