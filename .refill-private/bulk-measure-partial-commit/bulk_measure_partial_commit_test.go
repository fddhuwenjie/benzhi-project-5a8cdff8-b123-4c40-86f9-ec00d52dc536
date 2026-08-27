package bulkmeasurepartialcommit_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"seismocal/internal/calibration"
	"seismocal/internal/case_service"
	"seismocal/internal/model"
	"seismocal/internal/review"
	"seismocal/internal/storage"
	"seismocal/internal/web"
	"testing"
)

func TestRejectedBulkMeasurementDoesNotPartiallyCommit(t *testing.T) {
	store := storage.New(t.TempDir())
	cases := case_service.New(store)
	created, err := cases.Create("BULK-TX-1", "STA-01", "SEISMO-X", "SN-001", "engineer-a", "STD-01")
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	frozen, err := cases.Freeze(created.CaseID, created.Revision, model.Baseline{
		TemperatureC:    20,
		HumidityPercent: 40,
		GaugeConfig:     `"GAUGE-01"`,
	})
	if err != nil {
		t.Fatalf("freeze case: %v", err)
	}

	server := web.New(cases, calibration.New(cases, store), review.New(cases, store), store)
	body := map[string]any{
		"expected_revision": frozen.Revision,
		"batches": []map[string]any{
			{"phase": "zero", "values": []float64{1, 1, 1}, "sample_count": 3, "temperature_c": 20, "humidity_percent": 40},
			{"phase": "frequency", "values": []float64{2, 2, 2}, "sample_count": 3, "temperature_c": 20, "humidity_percent": 40},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cases/BULK-TX-1/measure", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, req)
	if response.Code == http.StatusOK {
		t.Fatalf("late-invalid bulk request unexpectedly succeeded: %s", response.Body.String())
	}

	after, ok := store.GetCase(created.CaseID)
	if !ok {
		t.Fatal("case disappeared after rejected bulk request")
	}
	if after.Revision != frozen.Revision {
		t.Fatalf("rejected bulk request changed revision: before=%d after=%d", frozen.Revision, after.Revision)
	}
	if len(after.Batches) != 0 {
		t.Fatalf("rejected bulk request partially committed %d batch(es)", len(after.Batches))
	}
	for _, event := range store.Audit(created.CaseID) {
		if event.Action == "measurement_recorded" {
			t.Fatalf("rejected bulk request appended audit event %s", event.ID)
		}
	}
}
