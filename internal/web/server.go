package web

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"strings"
	"sync"
	"time"
)

type Server struct {
	cases     *case_service.Service
	calib     *calibration.Service
	review    *review.Service
	store     *storage.Store
	mux       *http.ServeMux
	requestMu sync.Mutex
}

func New(c *case_service.Service, cal *calibration.Service, r *review.Service, s *storage.Store) *Server {
	sv := &Server{cases: c, calib: cal, review: r, store: s, mux: http.NewServeMux()}
	sv.routes()
	return sv
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) routes() {
	s.mux.HandleFunc("/ui/calibration", s.ui)
	s.mux.HandleFunc("/ui/calibration.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte("body{font-family:system-ui;background:#f5f7fa}"))
	})
	s.mux.HandleFunc("/ui/calibration.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte("console.log('SeismoCal工作台已加载')"))
	})
	s.mux.HandleFunc("/api/v1/cases", s.casesAPI)
	s.mux.HandleFunc("/api/v1/cases/", s.caseAPI)
	s.mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) { jsonOK(w, map[string]string{"status": "ok"}) })
}
func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonErr(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json")
	status := http.StatusBadRequest
	if strings.Contains(e.Error(), "冲突") || strings.Contains(strings.ToLower(e.Error()), "conflict") {
		status = http.StatusConflict
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
}
func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func (s *Server) replay(w http.ResponseWriter, r *http.Request) bool {
	rid := r.Header.Get("X-Request-ID")
	if rid == "" {
		return false
	}
	if b, ok := s.store.GetRequest(rid); ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
		return true
	}
	return false
}
func (s *Server) remember(r *http.Request, v any) {
	if rid := r.Header.Get("X-Request-ID"); rid != "" {
		s.store.PutRequest(rid, v)
	}
}
func (s *Server) ui(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = template.Must(template.New("ui").Parse(uiHTML)).Execute(w, nil)
}

type createReq struct {
	CaseID                string `json:"case_id"`
	StationCode           string `json:"station_code"`
	InstrumentModel       string `json:"instrument_model"`
	SerialNumber          string `json:"serial_number"`
	ResponsibleEngineer   string `json:"responsible_engineer"`
	CalibrationStandardID string `json:"calibration_standard_id"`
}

func (s *Server) casesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.searchCases(w, r)
		return
	}
	if r.Method != "POST" {
		methodNotAllowed(w)
		return
	}
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	if s.replay(w, r) {
		return
	}
	var q createReq
	if err := decode(r, &q); err != nil {
		jsonErr(w, fmt.Errorf("请求格式错误: %w", err))
		return
	}
	c, e := s.cases.Create(q.CaseID, q.StationCode, q.InstrumentModel, q.SerialNumber, q.ResponsibleEngineer, q.CalibrationStandardID)
	if e != nil {
		jsonErr(w, e)
		return
	}
	s.remember(r, c)
	jsonOK(w, c)
}

func cursorFor(c *model.CalibrationCase) string {
	t := c.UpdatedAt
	if t.IsZero() {
		t = c.OpenedAt
	}
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + c.CaseID
	h := sha256.Sum256([]byte(raw + "|seismocal"))
	return base64.RawURLEncoding.EncodeToString([]byte(raw + "." + fmt.Sprintf("%x", h[:8])))
}
func (s *Server) searchCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := storage.CaseFilter{StationCode: q.Get("station_code"), SerialNumber: q.Get("serial_number"), ResponsibleEngineer: q.Get("responsible_engineer"), Limit: 20, Cursor: ""}
	if v := q.Get("status"); v != "" {
		st := model.CaseStatus(v)
		switch st {
		case model.StatusDraft, model.StatusMeasuring, model.StatusPaused, model.StatusReview, model.StatusReleased:
			f.Status = &st
		default:
			jsonErr(w, fmt.Errorf("status字段无效"))
			return
		}
	}
	if v := q.Get("page_size"); v != "" {
		var n int
		if _, e := fmt.Sscanf(v, "%d", &n); e != nil || n < 1 || n > 100 {
			jsonErr(w, fmt.Errorf("page_size字段无效"))
			return
		}
		f.Limit = n
	}
	if f.StationCode == "" && f.SerialNumber == "" && f.ResponsibleEngineer == "" && f.Status == nil && f.Limit > 50 {
		jsonErr(w, fmt.Errorf("空条件查询页大小超限"))
		return
	}
	if cur := q.Get("cursor"); cur != "" {
		b, e := base64.RawURLEncoding.DecodeString(cur)
		if e != nil {
			jsonErr(w, fmt.Errorf("cursor字段无效"))
			return
		}
		parts := strings.Split(string(b), ".")
		if len(parts) != 2 {
			jsonErr(w, fmt.Errorf("cursor字段无效"))
			return
		}
		h := sha256.Sum256([]byte(parts[0] + "|seismocal"))
		if parts[1] != fmt.Sprintf("%x", h[:8]) {
			jsonErr(w, fmt.Errorf("cursor字段无效"))
			return
		}
		f.Cursor = parts[0]
	}
	list, total, e := s.store.ListCases(f)
	if e != nil {
		jsonErr(w, e)
		return
	}
	counts := map[string]int{}
	for _, st := range []model.CaseStatus{model.StatusDraft, model.StatusMeasuring, model.StatusPaused, model.StatusReview, model.StatusReleased} {
		counts[string(st)] = 0
	}
	open := 0
	allCases, _, _ := s.store.ListCases(storage.CaseFilter{Limit: 100000})
	for _, c := range allCases {
		counts[string(c.Status)]++
		for _, a := range c.Anomalies {
			if !a.Resolved {
				open++
			}
		}
	}
	next := ""
	if len(list) == f.Limit {
		next = cursorFor(list[len(list)-1])
	}
	type item struct {
		*model.CalibrationCase
		NextAction string `json:"next_action"`
		Blocker    string `json:"blocker,omitempty"`
	}
	out := make([]item, 0, len(list))
	for _, c := range list {
		act, block := "待冻结", ""
		switch c.Status {
		case model.StatusMeasuring:
			act = "待测量"
		case model.StatusPaused:
			act = "异常待复测"
		case model.StatusReview:
			act = "待签发"
		case model.StatusReleased:
			act = "已放行"
		}
		if c.Status == model.StatusMeasuring && len(c.Batches) >= 3 {
			act = "待复核"
		}
		if len(calibration.OpenAnomalies(*c)) > 0 {
			block = "存在开放异常"
		}
		out = append(out, item{c, act, block})
	}
	jsonOK(w, map[string]any{"items": out, "total": total, "next_cursor": next, "summary": map[string]any{"counts": counts, "open_anomalies": open}})
}

type caseView struct {
	*model.CalibrationCase        `json:",inline"`
	Timeline                      []model.AuditEvent              `json:"timeline,omitempty"`
	AuditHead                     string                          `json:"audit_head,omitempty"`
	CertificateVerification       map[string]bool                 `json:"certificate_verification,omitempty"`
	CertificateVerificationReport *review.CertificateVerification `json:"certificate_verification_report,omitempty"`
	CompletedPhases               map[string]bool                 `json:"completed_phases,omitempty"`
	NextPhase                     string                          `json:"next_phase,omitempty"`
	QualityFailures               []string                        `json:"quality_failures,omitempty"`
}

func (s *Server) caseAPI(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/cases/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		w.WriteHeader(404)
		return
	}
	id := parts[0]
	if len(parts) == 2 && parts[1] == "certificate-verification" {
		if err := model.ValidateIdentifier("case_id", id); err != nil {
			jsonErr(w, err)
			return
		}
		if r.Method != "GET" {
			methodNotAllowed(w)
			return
		}
	}
	if r.Method == "GET" {
		c, ok := s.store.GetCase(id)
		if !ok {
			w.WriteHeader(404)
			return
		}
		if len(parts) == 2 && parts[1] == "issue-check" {
			v, e := s.review.IssueCheck(id)
			if e != nil {
				jsonErr(w, e)
			} else {
				jsonOK(w, v)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "measurement-summary" {
			phase := r.URL.Query().Get("phase")
			if phase != "" && phase != "zero" && phase != "sensitivity" && phase != "frequency" {
				jsonErr(w, fmt.Errorf("phase字段无效"))
				return
			}
			var from, to time.Time
			var e error
			if v := r.URL.Query().Get("from"); v != "" {
				from, e = time.Parse(time.RFC3339, v)
				if e != nil {
					jsonErr(w, fmt.Errorf("from字段无效"))
					return
				}
			}
			if v := r.URL.Query().Get("to"); v != "" {
				to, e = time.Parse(time.RFC3339, v)
				if e != nil {
					jsonErr(w, fmt.Errorf("to字段无效"))
					return
				}
			}
			if !from.IsZero() && !to.IsZero() && from.After(to) {
				jsonErr(w, fmt.Errorf("时间范围无效"))
				return
			}
			jsonOK(w, calibration.Summarize(*c, phase, from, to))
			return
		}
		if len(parts) == 2 && parts[1] == "certificate-preview" {
			v, e := s.review.Preview(id)
			if e != nil {
				jsonErr(w, e)
			} else {
				jsonOK(w, v)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "certificate-verification" {
			v, e := s.review.VerifyCertificate(id)
			if e != nil {
				jsonErr(w, e)
			} else {
				jsonOK(w, v)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "evidence-manifest" {
			v, e := s.store.EvidenceManifest(id)
			if e != nil {
				jsonErr(w, e)
			} else {
				jsonOK(w, v)
			}
			return
		}
		if len(parts) == 2 && parts[1] == "evidence" {
			b, e := s.store.Evidence(id)
			if e != nil {
				jsonErr(w, e)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+id+"-evidence.json\"")
			_, _ = w.Write(b)
			return
		}
		var from, to time.Time
		if v := r.URL.Query().Get("from"); v != "" {
			from, _ = time.Parse(time.RFC3339, v)
		}
		if v := r.URL.Query().Get("to"); v != "" {
			to, _ = time.Parse(time.RFC3339, v)
		}
		events := s.store.Timeline(id, r.URL.Query().Get("action"), from, to)
		head := ""
		if len(s.store.Audit(id)) > 0 {
			a := s.store.Audit(id)
			head = a[len(a)-1].Digest
		}
		ver := map[string]bool{}
		if c.Certificate != nil {
			ver["certificate_digest_match"] = c.Certificate.CertificateDigest == review.CertificateDigest(*c)
			ver["case_status_match"] = c.Status == model.StatusReleased
			ver["audit_head_match"] = c.Certificate.AuditHead == head
		}
		ph := calibration.CompletedPhases(*c)
		next := ""
		for _, p := range []string{"zero", "sensitivity", "frequency"} {
			if !ph[p] {
				next = p
				break
			}
		}
		fails := []string{}
		for _, b := range c.Batches {
			if b.QualityState == "failed" {
				fails = append(fails, b.Quality.Code)
			}
		}
		report, _ := s.review.VerifyCertificate(id)
		jsonOK(w, caseView{CalibrationCase: c, Timeline: events, AuditHead: head, CertificateVerification: ver, CertificateVerificationReport: &report, CompletedPhases: ph, NextPhase: next, QualityFailures: fails})
		return
	}
	if r.Method != "POST" || len(parts) < 2 {
		methodNotAllowed(w)
		return
	}
	if parts[1] != "issue" && s.replay(w, r) {
		return
	}
	switch parts[1] {
	case "freeze":
		var q struct {
			ExpectedRevision int     `json:"expected_revision"`
			TemperatureC     float64 `json:"temperature_c"`
			HumidityPercent  float64 `json:"humidity_percent"`
			GaugeConfig      string  `json:"gauge_config"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		c, e := s.cases.Freeze(id, q.ExpectedRevision, model.Baseline{TemperatureC: q.TemperatureC, HumidityPercent: q.HumidityPercent, GaugeConfig: q.GaugeConfig})
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	case "measure":
		var q struct {
			ExpectedRevision int                      `json:"expected_revision"`
			Phase            string                   `json:"phase"`
			Values           []float64                `json:"values"`
			TemperatureC     float64                  `json:"temperature_c"`
			HumidityPercent  float64                  `json:"humidity_percent"`
			SampleCount      int                      `json:"sample_count"`
			SampleDigest     string                   `json:"sample_digest"`
			Batches          []model.MeasurementBatch `json:"batches"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		if len(q.Batches) > 0 {
			c, res, e := s.calib.AddBatches(id, q.ExpectedRevision, q.Batches)
			if e != nil {
				jsonErr(w, e)
				return
			}
			resp := map[string]any{"case": c, "results": res, "revision": c.Revision}
			s.remember(r, resp)
			jsonOK(w, resp)
			return
		}
		if q.SampleDigest != "" {
			h := sha256.New()
			for _, v := range q.Values {
				fmt.Fprintf(h, "%.8f,", v)
			}
			if fmt.Sprintf("%x", h.Sum(nil)) != q.SampleDigest {
				jsonErr(w, fmt.Errorf("sample_digest不匹配"))
				return
			}
		}
		select {
		case <-r.Context().Done():
			jsonErr(w, fmt.Errorf("请求已取消: %w", r.Context().Err()))
			return
		default:
		}
		c, e := s.calib.AddBatch(id, q.ExpectedRevision, model.MeasurementBatch{Phase: q.Phase, Values: q.Values, SampleCount: q.SampleCount, TemperatureC: q.TemperatureC, HumidityPercent: q.HumidityPercent})
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	case "retest":
		var q struct {
			ExpectedRevision   int       `json:"expected_revision"`
			Phase              string    `json:"phase"`
			Values             []float64 `json:"values"`
			SampleCount        int       `json:"sample_count"`
			TemperatureC       float64   `json:"temperature_c"`
			HumidityPercent    float64   `json:"humidity_percent"`
			Disposition        string    `json:"disposition"`
			BatchID            string    `json:"batch_id"`
			AnomalyID          string    `json:"anomaly_id"`
			DispositionDetails string    `json:"disposition_details"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		if q.AnomalyID != "" {
			matched := false
			if cc, ok := s.store.GetCase(id); ok {
				for _, a := range cc.Anomalies {
					if a.AnomalyID == q.AnomalyID {
						q.BatchID = a.BatchID
						matched = true
					}
				}
			}
			if !matched {
				jsonErr(w, fmt.Errorf("异常标识不存在"))
				return
			}
		}
		if strings.TrimSpace(q.DispositionDetails) != "" {
			q.Disposition = strings.TrimSpace(q.Disposition) + ": " + strings.TrimSpace(q.DispositionDetails)
		}
		c, e := s.calib.ResolveAnomaly(id, q.ExpectedRevision, model.MeasurementBatch{Phase: q.Phase, Values: q.Values, SampleCount: q.SampleCount, TemperatureC: q.TemperatureC, HumidityPercent: q.HumidityPercent, BatchID: q.BatchID}, q.Disposition)
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	case "review":
		var q struct {
			ExpectedRevision int    `json:"expected_revision"`
			ReviewerID       string `json:"reviewer_id"`
			ReviewerRole     string `json:"reviewer_role"`
			Decision         string `json:"decision"`
			Rationale        string `json:"rationale"`
			EvidenceDigest   string `json:"evidence_digest"`
			RejectCategory   string `json:"reject_category"`
			AffectedPhase    string `json:"affected_phase"`
			AffectedAnomaly  string `json:"affected_anomaly"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		c, e := s.review.Submit(id, q.ExpectedRevision, model.ReviewDecision{ReviewerID: q.ReviewerID, ReviewerRole: q.ReviewerRole, Decision: q.Decision, Rationale: q.Rationale, EvidenceDigest: q.EvidenceDigest, RejectCategory: q.RejectCategory, AffectedPhase: q.AffectedPhase, AffectedAnomaly: q.AffectedAnomaly})
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	case "issue":
		var q struct {
			ExpectedRevision  int    `json:"expected_revision"`
			IssuedBy          string `json:"issued_by"`
			CertificateDigest string `json:"certificate_digest"`
			ExpectedDigest    string `json:"expected_digest"`
			PreviewLock       string `json:"preview_lock"`
			Lock              string `json:"lock"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		if c, ok := s.store.GetCase(id); ok && c.Certificate != nil {
			jsonOK(w, c)
			return
		}
		if q.CertificateDigest == "" {
			q.CertificateDigest = q.ExpectedDigest
		}
		if q.PreviewLock == "" {
			q.PreviewLock = q.Lock
		}
		c, e := s.review.Issue(id, q.ExpectedRevision, q.IssuedBy, q.CertificateDigest, q.PreviewLock)
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	case "remediation":
		var q struct {
			ExpectedRevision int      `json:"expected_revision"`
			ItemID           string   `json:"item_id"`
			Explanation      string   `json:"explanation"`
			EvidenceBatchIDs []string `json:"evidence_batch_ids"`
		}
		if decode(r, &q) != nil {
			jsonErr(w, fmt.Errorf("请求格式错误"))
			return
		}
		c, e := s.review.UpdateRemediation(id, q.ExpectedRevision, q.ItemID, q.Explanation, q.EvidenceBatchIDs)
		if e != nil {
			jsonErr(w, e)
			return
		}
		s.remember(r, c)
		jsonOK(w, c)
	default:
		w.WriteHeader(404)
	}
}

const uiHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><title>SeismoCal校准放行台</title><style>body{font-family:system-ui;margin:2rem;background:#f5f7fa;color:#203040}main{max-width:900px;margin:auto;background:white;padding:2rem;border-radius:12px}button{padding:.6rem 1rem;background:#1769aa;color:white;border:0;border-radius:4px}input{padding:.5rem;margin:.2rem}pre{background:#eef2f6;padding:1rem;overflow:auto}</style></head><body><main><h1>SeismoCal地震台站仪器校准放行台</h1><p>通过同源 JSON API 管理校准案件、测量、异常复测、独立复核和证书封存。</p><section><h2>案件检索与待办看板</h2><input id="qstation" placeholder="台站代码"><input id="qserial" placeholder="仪器序列号"><input id="qstatus" placeholder="状态"><button onclick="searchCases()">查询案件</button></section><section><h2>创建校准案件</h2><input id="case" placeholder="案件编号"><input id="station" placeholder="台站代码"><input id="serial" placeholder="仪器序列号"><input id="engineer" placeholder="责任工程师"><button onclick="create()">创建并查看</button><button onclick="summary()">查看阶段统计</button><button onclick="verifyCertificate()">证书巡检</button></section><pre id="out">等待操作…</pre></main><script>async function searchCases(){let p=new URLSearchParams();[['station_code','qstation'],['serial_number','qserial'],['status','qstatus']].forEach(x=>{let v=document.getElementById(x[1]).value;if(v)p.set(x[0],v)});let r=await fetch('/api/v1/cases?'+p);document.getElementById('out').textContent=JSON.stringify(await r.json(),null,2)} async function create(){let body={case_id:document.getElementById('case').value,station_code:document.getElementById('station').value,serial_number:document.getElementById('serial').value,responsible_engineer:document.getElementById('engineer').value,instrument_model:'Seismo-标准',calibration_standard_id:'STD-001'};let r=await fetch('/api/v1/cases',{method:'POST',headers:{'Content-Type':'application/json','X-Request-ID':crypto.randomUUID()},body:JSON.stringify(body)});document.getElementById('out').textContent=JSON.stringify(await r.json(),null,2)} async function summary(){let id=document.getElementById('case').value;let r=await fetch('/api/v1/cases/'+encodeURIComponent(id)+'/measurement-summary');document.getElementById('out').textContent=JSON.stringify(await r.json(),null,2)} async function verifyCertificate(){let id=document.getElementById('case').value.trim();if(!id)return;let r=await fetch('/api/v1/cases/'+encodeURIComponent(id)+'/certificate-verification');document.getElementById('out').textContent=JSON.stringify(await r.json(),null,2)}</script></body></html>`
