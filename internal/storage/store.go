package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"seismocal/internal/model"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu             sync.RWMutex
	dir            string
	cases          map[string]*model.CalibrationCase
	requests       map[string][]byte
	audit          map[string][]model.AuditEvent
	evidence       map[string][]byte
	evidenceDigest map[string]string
	inspection     map[string]model.AuditInspection
}

type CaseFilter struct {
	StationCode, SerialNumber, ResponsibleEngineer string
	Status                                         *model.CaseStatus
	Limit                                          int
	Cursor                                         string
}

func New(dir string) *Store {
	s := &Store{dir: dir, cases: map[string]*model.CalibrationCase{}, requests: map[string][]byte{}, audit: map[string][]model.AuditEvent{}, evidence: map[string][]byte{}, evidenceDigest: map[string]string{}, inspection: map[string]model.AuditInspection{}}
	_ = os.MkdirAll(dir, 0755)
	if es, e := os.ReadDir(dir); e == nil {
		for _, x := range es {
			if strings.HasPrefix(x.Name(), "evidence-") && filepath.Ext(x.Name()) == ".json" {
				if b, e := os.ReadFile(filepath.Join(dir, x.Name())); e == nil {
					key := strings.TrimSuffix(strings.TrimPrefix(x.Name(), "evidence-"), ".json")
					s.evidence[key] = b
					if s.evidenceDigest[key] == "" {
						sum := sha256.Sum256(b)
						s.evidenceDigest[key] = hex.EncodeToString(sum[:])
					}
				}
				continue
			}
			if strings.HasPrefix(x.Name(), "evidence-") && filepath.Ext(x.Name()) == ".sha256" {
				if b, e := os.ReadFile(filepath.Join(dir, x.Name())); e == nil {
					key := strings.TrimSuffix(strings.TrimPrefix(x.Name(), "evidence-"), ".sha256")
					s.evidenceDigest[key] = strings.TrimSpace(string(b))
				}
				continue
			}
			if filepath.Ext(x.Name()) == ".audit" {
				b, e := os.ReadFile(filepath.Join(dir, x.Name()))
				if e == nil {
					var a []model.AuditEvent
					if json.Unmarshal(b, &a) == nil {
						s.audit[strings.TrimSuffix(x.Name(), ".audit")] = a
						s.inspection[strings.TrimSuffix(x.Name(), ".audit")] = inspectAudit(a)
					}
				}
				continue
			}
			if filepath.Ext(x.Name()) != ".json" {
				continue
			}
			b, e := os.ReadFile(filepath.Join(dir, x.Name()))
			if e != nil {
				continue
			}
			var c model.CalibrationCase
			if json.Unmarshal(b, &c) == nil && c.CaseID != "" {
				s.cases[c.CaseID] = cloneCase(&c)
			}
		}
	}
	return s
}

func inspectAudit(events []model.AuditEvent) model.AuditInspection {
	r := model.AuditInspection{Healthy: true, CheckedAt: time.Now().UTC()}
	prev := ""
	for _, e := range events {
		raw, _ := json.Marshal(e.Data)
		sum := sha256.Sum256(append([]byte(prev+e.Action+e.At.Format(time.RFC3339Nano)), raw...))
		if e.PreviousDigest != prev || e.Digest != hex.EncodeToString(sum[:]) {
			r.Healthy = false
			r.BrokenEventID = e.ID
			r.Reason = "前序摘要或事件摘要不匹配"
			break
		}
		prev = e.Digest
	}
	return r
}
func (s *Store) AuditInspection(caseID string) model.AuditInspection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if x, ok := s.inspection[caseID]; ok {
		return x
	}
	return model.AuditInspection{Healthy: true, CheckedAt: time.Now().UTC()}
}
func (s *Store) InspectAudits() map[string]model.AuditInspection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]model.AuditInspection{}
	for id, a := range s.audit {
		out[id] = inspectAudit(a)
	}
	return out
}

func (s *Store) ListCases(f CaseFilter) ([]*model.CalibrationCase, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*model.CalibrationCase, 0)
	for _, c := range s.cases {
		if f.StationCode != "" && c.StationCode != f.StationCode {
			continue
		}
		if f.SerialNumber != "" && c.SerialNumber != f.SerialNumber {
			continue
		}
		if f.ResponsibleEngineer != "" && c.ResponsibleEngineer != f.ResponsibleEngineer {
			continue
		}
		if f.Status != nil && c.Status != *f.Status {
			continue
		}
		all = append(all, cloneCase(c))
	}
	sort.Slice(all, func(i, j int) bool {
		ti, tj := all[i].UpdatedAt, all[j].UpdatedAt
		if ti.IsZero() {
			ti = all[i].OpenedAt
		}
		if tj.IsZero() {
			tj = all[j].OpenedAt
		}
		if ti.Equal(tj) {
			return all[i].CaseID < all[j].CaseID
		}
		return ti.After(tj)
	})
	start := 0
	if f.Cursor != "" {
		for i, c := range all {
			t := c.UpdatedAt
			if t.IsZero() {
				t = c.OpenedAt
			}
			key := t.UTC().Format(time.RFC3339Nano) + "|" + c.CaseID
			if key == f.Cursor {
				start = i + 1
				break
			}
		}
		if start == 0 {
			return nil, 0, fmt.Errorf("cursor无效")
		}
	}
	total := len(all)
	if f.Limit <= 0 {
		f.Limit = 20
	}
	end := start + f.Limit
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], total, nil
}
func cloneCase(c *model.CalibrationCase) *model.CalibrationCase {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
	var out model.CalibrationCase
	_ = json.Unmarshal(b, &out)
	return &out
}
func (s *Store) SaveCase(c *model.CalibrationCase) error {
	if c == nil {
		return fmt.Errorf("案件为空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneCase(c)
	b, e := json.Marshal(cp)
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.dir, c.CaseID+".tmp")
	if e = os.WriteFile(tmp, b, 0644); e == nil {
		e = os.Rename(tmp, filepath.Join(s.dir, c.CaseID+".json"))
	}
	if e != nil {
		// Only the durable copy authorizes visibility: leave the in-memory
		// state at its last successfully persisted value so failed writes do
		// not pollute queries, listings or duplicate-creation checks.
		return e
	}
	s.cases[c.CaseID] = cp
	return nil
}
func (s *Store) GetCase(id string) (*model.CalibrationCase, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cases[id]
	out := cloneCase(c)
	if ok {
		in := s.inspection[id]
		out.AuditInspection = &in
	}
	return out, ok
}
func (s *Store) FindActiveByInstrument(station, serial string) (*model.CalibrationCase, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cases {
		if c.StationCode == station && c.SerialNumber == serial && c.Status != model.StatusReleased {
			return cloneCase(c), true
		}
	}
	return nil, false
}
func (s *Store) PutRequest(id string, v any) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(v)
	s.requests[id] = append([]byte(nil), b...)
}
func (s *Store) GetRequest(id string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.requests[id]
	return append([]byte(nil), b...), ok
}
func (s *Store) AppendAudit(caseID, action string, data map[string]string) model.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := ""
	if a := s.audit[caseID]; len(a) > 0 {
		prev = a[len(a)-1].Digest
	}
	if data == nil {
		data = map[string]string{}
	}
	ev := model.AuditEvent{ID: fmt.Sprintf("%s-%d", caseID, len(s.audit[caseID])+1), CaseID: caseID, Action: action, PreviousDigest: prev, At: time.Now().UTC(), Data: data}
	raw, _ := json.Marshal(data)
	sum := sha256.Sum256(append([]byte(prev+action+ev.At.Format(time.RFC3339Nano)), raw...))
	ev.Digest = hex.EncodeToString(sum[:])
	s.audit[caseID] = append(s.audit[caseID], ev)
	s.inspection[caseID] = model.AuditInspection{Healthy: true, CheckedAt: time.Now().UTC()}
	if b, e := json.Marshal(s.audit[caseID]); e == nil {
		_ = os.WriteFile(filepath.Join(s.dir, caseID+".audit"), b, 0644)
	}
	return ev
}
func (s *Store) Audit(caseID string) []model.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]model.AuditEvent(nil), s.audit[caseID]...)
	for i := range out {
		if out[i].Data != nil {
			out[i].Data = map[string]string{}
			for k, v := range s.audit[caseID][i].Data {
				out[i].Data[k] = v
			}
		}
	}
	return out
}
func (s *Store) Evidence(caseID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.cases[caseID]
	if !ok {
		return nil, fmt.Errorf("case not found")
	}
	if in := s.inspection[caseID]; !in.Healthy {
		return nil, fmt.Errorf("审计巡检失败: %s (%s)", in.BrokenEventID, in.Reason)
	}
	if c.Status != model.StatusReleased || c.Certificate == nil {
		return nil, fmt.Errorf("案件尚未放行，无法生成证据包")
	}
	events := append([]model.AuditEvent(nil), s.audit[caseID]...)
	if !VerifyAuditChain(events) {
		return nil, fmt.Errorf("审计链完整性校验失败")
	}
	headNow := ""
	if len(events) > 0 {
		headNow = events[len(events)-1].Digest
	}
	if c.Certificate.AuditHead != "" && c.Certificate.AuditHead != headNow {
		return nil, fmt.Errorf("证书审计头不一致")
	}
	if b, ok := s.evidence[caseID]; ok {
		sum := sha256.Sum256(b)
		if s.evidenceDigest[caseID] != "" && s.evidenceDigest[caseID] != hex.EncodeToString(sum[:]) {
			return nil, fmt.Errorf("证据包完整性校验失败")
		}
		return append([]byte(nil), b...), nil
	}
	sort.SliceStable(c.Batches, func(i, j int) bool { return c.Batches[i].RecordedAt.Before(c.Batches[j].RecordedAt) })
	head := ""
	if len(events) > 0 {
		head = events[len(events)-1].Digest
	}
	payload := struct {
		Case      *model.CalibrationCase `json:"case"`
		Audit     []model.AuditEvent     `json:"audit"`
		AuditHead string                 `json:"audit_head"`
		Manifest  map[string]int         `json:"manifest"`
	}{cloneCase(c), events, head, map[string]int{"case": 1, "baseline": 1, "batches": len(c.Batches), "anomalies": len(c.Anomalies), "reviews": len(c.Reviews), "remediations": len(c.Remediations), "audit": len(events)}}
	b, e := json.Marshal(payload)
	if e != nil {
		return nil, e
	}
	s.evidence[caseID] = append([]byte(nil), b...)
	sum := sha256.Sum256(b)
	s.evidenceDigest[caseID] = hex.EncodeToString(sum[:])
	_ = os.WriteFile(filepath.Join(s.dir, "evidence-"+caseID+".json"), b, 0644)
	_ = os.WriteFile(filepath.Join(s.dir, "evidence-"+caseID+".sha256"), []byte(s.evidenceDigest[caseID]), 0644)
	return b, nil
}

// EvidenceSnapshot returns the currently archived evidence bytes and their
// sidecar digest without generating or persisting anything. It is used by
// read-only certificate verification so a missing archive is reported rather
// than silently repaired.
func (s *Store) EvidenceSnapshot(caseID string) ([]byte, string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, err := os.ReadFile(filepath.Join(s.dir, "evidence-"+caseID+".json")); err == nil {
		digest := s.evidenceDigest[caseID]
		if sidecar, e := os.ReadFile(filepath.Join(s.dir, "evidence-"+caseID+".sha256")); e == nil {
			digest = strings.TrimSpace(string(sidecar))
		}
		return b, digest, true
	}
	if s.dir != "" {
		return nil, "", false
	}
	b, ok := s.evidence[caseID]
	if !ok {
		return nil, s.evidenceDigest[caseID], false
	}
	return append([]byte(nil), b...), s.evidenceDigest[caseID], true
}

// AuditSnapshot rereads the persisted audit file when available. The bool is
// false when the file exists but cannot be decoded, allowing callers to report
// a broken chain instead of silently using a stale in-memory copy.
func (s *Store) AuditSnapshot(caseID string) ([]model.AuditEvent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if b, err := os.ReadFile(filepath.Join(s.dir, caseID+".audit")); err == nil {
		var events []model.AuditEvent
		if json.Unmarshal(b, &events) != nil {
			return nil, false
		}
		return events, true
	}
	if s.dir != "" {
		return nil, false
	}
	events, ok := s.audit[caseID]
	if !ok {
		return nil, false
	}
	return append([]model.AuditEvent(nil), events...), true
}

func (s *Store) EvidenceManifest(caseID string) (map[string]any, error) {
	if in := s.AuditInspection(caseID); !in.Healthy {
		return nil, fmt.Errorf("审计巡检失败: %s (%s)", in.BrokenEventID, in.Reason)
	}
	b, e := s.Evidence(caseID)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(b)
	c, ok := s.GetCase(caseID)
	if !ok || c.Certificate == nil {
		return nil, fmt.Errorf("案件不存在")
	}
	return map[string]any{"case_id": caseID, "certificate_digest": c.Certificate.CertificateDigest, "content_digest": hex.EncodeToString(sum[:]), "inspection": s.AuditInspection(caseID), "components": map[string]any{"case": map[string]any{"count": 1}, "baseline": map[string]any{"count": 1, "digest": c.Baseline.Fingerprint}, "batches": map[string]any{"count": len(c.Batches)}, "anomalies": map[string]any{"count": len(c.Anomalies)}, "reviews": map[string]any{"count": len(c.Reviews)}, "remediations": map[string]any{"count": len(c.Remediations)}, "audit": map[string]any{"count": len(s.Audit(caseID)), "head": c.Certificate.AuditHead}}}, nil
}
func (s *Store) Timeline(caseID, action string, from, to time.Time) []model.AuditEvent {
	events := s.Audit(caseID)
	out := make([]model.AuditEvent, 0, len(events))
	for _, e := range events {
		if action != "" && e.Action != action {
			continue
		}
		if !from.IsZero() && e.At.Before(from) {
			continue
		}
		if !to.IsZero() && e.At.After(to) {
			continue
		}
		out = append(out, e)
	}
	return out
}
