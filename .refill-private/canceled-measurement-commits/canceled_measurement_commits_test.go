package canceledmeasurementcommits

import (
	"context"
	"net/http/httptest"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"seismocal/internal/web"
	"strings"
	"testing"
	"time"
)

func TestCanceledMeasurementDoesNotCommit(t *testing.T) {
	store := storage.New(t.TempDir())
	now := time.Now().UTC()
	if err := store.SaveCase(&model.CalibrationCase{
		CaseID:                "CANCEL-MEASURE-1",
		StationCode:           "STA-01",
		InstrumentModel:       "SEISMO-X",
		SerialNumber:          "SN-01",
		ResponsibleEngineer:   "engineer-1",
		CalibrationStandardID: "STD-01",
		Status:                model.StatusMeasuring,
		Revision:              7,
		OpenedAt:              now,
		UpdatedAt:             now,
		Baseline: model.Baseline{
			TemperatureC:    20,
			HumidityPercent: 50,
			GaugeConfig:     `[{"gauge_id":"G-1"}]`,
			Fingerprint:     "baseline-digest",
			FrozenAt:        now,
		},
	}); err != nil {
		t.Fatalf("准备案件失败: %v", err)
	}

	cases := case_service.New(store)
	server := web.New(cases, calibration.New(cases, store), review.New(cases, store), store)
	body := `{"expected_revision":7,"phase":"zero","values":[1,1,1],"sample_count":3,"temperature_c":20,"humidity_percent":50}`
	request := httptest.NewRequest("POST", "/api/v1/cases/CANCEL-MEASURE-1/measure", strings.NewReader(body))
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code == 200 {
		t.Fatalf("已取消请求错误返回成功: body=%s", response.Body.String())
	}
	got, ok := store.GetCase("CANCEL-MEASURE-1")
	if !ok {
		t.Fatal("案件意外丢失")
	}
	audit := store.Audit("CANCEL-MEASURE-1")
	if got.Revision != 7 || len(got.Batches) != 0 || len(audit) != 0 {
		t.Fatalf("已取消请求仍提交测量: revision=%d batches=%d audit=%d", got.Revision, len(got.Batches), len(audit))
	}
}
