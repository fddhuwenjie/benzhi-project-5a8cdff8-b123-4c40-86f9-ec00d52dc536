package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"seismocal/internal/web"
	"time"
)

func listenAddr() string {
	a := flag.Lookup("addr")
	if a != nil && a.Value.String() != "" {
		return a.Value.String()
	}
	if p := os.Getenv("PORT"); p != "" {
		return "127.0.0.1:" + p
	}
	return "127.0.0.1:19081"
}
func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	self := flag.Bool("self-check", false, "执行完整流程后退出")
	flag.Parse()
	if *addr == "127.0.0.1:19081" {
		*addr = listenAddr()
	}
	if !safeAddress(*addr) {
		fmt.Fprintln(os.Stderr, "监听地址必须是回环地址")
		os.Exit(2)
	}
	st := storage.New(".seismocal-data")
	// 启动恢复后立即完成有界审计巡检；结果由只读证据接口暴露。
	_ = st.InspectAudits()
	cs := case_service.New(st)
	cal := calibration.New(cs, st)
	rv := review.New(cs, st)
	srv := web.New(cs, cal, rv, st)
	if *self {
		if err := selfCheck(cs, cal, rv, st); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	httpServer := &http.Server{Addr: *addr, Handler: srv.Handler(), ReadHeaderTimeout: 5 * time.Second}
	fmt.Println("SeismoCal服务监听 " + *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func selfCheck(cs *case_service.Service, cal *calibration.Service, rv *review.Service, st *storage.Store) error {
	id := fmt.Sprintf("SC-SELF-%d", time.Now().UnixNano())
	c, e := cs.Create(id, "STA-01", "Seismo-标准", "SN-001", "engineer-a", "STD-001")
	if e != nil {
		return e
	}
	c, e = cs.Freeze(c.CaseID, c.Revision, model.Baseline{TemperatureC: 22, HumidityPercent: 45, GaugeConfig: `{"gauge":"G-1"}`})
	if e != nil {
		return e
	}
	for _, phase := range []string{"zero", "sensitivity", "frequency"} {
		c, e = cal.AddBatch(c.CaseID, c.Revision, model.MeasurementBatch{Phase: phase, Values: []float64{1, 1.1, 0.9}, SampleCount: 3, TemperatureC: 22, HumidityPercent: 45})
		if e != nil {
			return e
		}
	}
	c, e = rv.Submit(c.CaseID, c.Revision, model.ReviewDecision{ReviewerID: "expert-b", ReviewerRole: "reviewer", Decision: "approve", Rationale: "数据完整", EvidenceDigest: review.EvidenceDigest(*c)})
	if e != nil {
		return e
	}
	c, e = rv.Issue(c.CaseID, c.Revision, "quality-admin")
	if e != nil {
		return e
	}
	if c.Status != model.StatusReleased || c.Certificate == nil {
		return fmt.Errorf("证书未签发")
	}
	if _, e = st.Evidence(c.CaseID); e != nil {
		return e
	}
	return nil
}
