package case_snapshot_alias_pollution_test

import (
	"testing"

	"seismocal/internal/model"
	"seismocal/internal/storage"
)

func TestGetCaseSnapshotMutationDoesNotPolluteStore(t *testing.T) {
	dir := t.TempDir()
	initial := &model.CalibrationCase{
		CaseID:   "SC-ALIAS-001",
		Status:   model.StatusMeasuring,
		Revision: 4,
		Baseline: model.Baseline{GaugeWarnings: []string{"量具临近到期: G-01"}},
		Batches: []model.MeasurementBatch{{
			BatchID: "B-001",
			Values:  []float64{0.98, 1.00, 1.02},
		}},
		Anomalies: []model.Anomaly{{
			AnomalyID: "AN-001",
			Attempts:  []model.RetestAttempt{{AttemptNo: 1, BatchID: "B-RETEST-001"}},
		}},
		Remediations: []model.RemediationItem{{
			ItemID:           "REM-001",
			EvidenceBatchIDs: []string{"B-001"},
		}},
		EnvironmentTrend: model.EnvironmentTrend{
			TriggerBatchIDs: []string{"B-001"},
			Phases: map[string]model.EnvironmentPhaseTrend{
				"zero": {BatchCount: 1},
			},
		},
	}
	if err := storage.New(dir).SaveCase(initial); err != nil {
		t.Fatalf("保存测试案件失败: %v", err)
	}

	// 从磁盘重启恢复，排除 SaveCase 入参本身的别名，只检验读取快照边界。
	st := storage.New(dir)
	view, ok := st.GetCase(initial.CaseID)
	if !ok {
		t.Fatal("重启后案件不存在")
	}
	view.Batches[0].Values[0] = 99
	view.Anomalies[0].Attempts[0].BatchID = "B-FORGED-RETEST"
	view.Remediations[0].EvidenceBatchIDs[0] = "B-FORGED-EVIDENCE"
	view.EnvironmentTrend.Phases["zero"] = model.EnvironmentPhaseTrend{BatchCount: 99}

	again, ok := st.GetCase(initial.CaseID)
	if !ok {
		t.Fatal("二次读取案件不存在")
	}
	if again.Batches[0].Values[0] != 0.98 ||
		again.Anomalies[0].Attempts[0].BatchID != "B-RETEST-001" ||
		again.Remediations[0].EvidenceBatchIDs[0] != "B-001" ||
		again.EnvironmentTrend.Phases["zero"].BatchCount != 1 {
		t.Fatalf("TestGetCaseSnapshotMutationDoesNotPolluteStore: 调用方修改读取快照后污染了 Store 内存聚合: values=%v attempts=%v remediation_evidence=%v phases=%v",
			again.Batches[0].Values, again.Anomalies[0].Attempts, again.Remediations[0].EvidenceBatchIDs, again.EnvironmentTrend.Phases)
	}
}
