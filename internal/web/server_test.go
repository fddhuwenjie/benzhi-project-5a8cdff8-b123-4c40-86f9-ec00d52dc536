package web

import (
	"net/http/httptest"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"strings"
	"testing"
)

func TestCreateAndGet(t *testing.T) {
	st := storage.New(t.TempDir())
	cs := case_service.New(st)
	sv := New(cs, calibration.New(cs, st), review.New(cs, st), st)
	r := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/cases", strings.NewReader(`{"case_id":"T1","station_code":"S","serial_number":"N","responsible_engineer":"e"}`))
	req.Header.Set("Content-Type", "application/json")
	sv.Handler().ServeHTTP(r, req)
	if r.Code != 200 {
		t.Fatalf("code %d", r.Code)
	}
	r = httptest.NewRecorder()
	sv.Handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/v1/cases/T1", nil))
	if r.Code != 200 {
		t.Fatalf("get %d", r.Code)
	}
}
