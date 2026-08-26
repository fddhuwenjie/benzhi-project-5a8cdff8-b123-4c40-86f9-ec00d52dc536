package model

import "time"

type CaseStatus string

const (
	StatusDraft     CaseStatus = "draft"
	StatusMeasuring CaseStatus = "measuring"
	StatusPaused    CaseStatus = "paused"
	StatusReview    CaseStatus = "review"
	StatusReleased  CaseStatus = "released"
)

type CalibrationCase struct {
	CaseID                string              `json:"case_id"`
	StationCode           string              `json:"station_code"`
	InstrumentModel       string              `json:"instrument_model"`
	SerialNumber          string              `json:"serial_number"`
	ResponsibleEngineer   string              `json:"responsible_engineer"`
	CalibrationStandardID string              `json:"calibration_standard_id"`
	Status                CaseStatus          `json:"status"`
	Revision              int                 `json:"revision"`
	OpenedAt              time.Time           `json:"opened_at"`
	UpdatedAt             time.Time           `json:"updated_at"`
	SealedAt              time.Time           `json:"sealed_at,omitempty"`
	Baseline              Baseline            `json:"baseline"`
	Batches               []MeasurementBatch  `json:"batches"`
	Anomalies             []Anomaly           `json:"anomalies"`
	Reviews               []ReviewDecision    `json:"reviews"`
	Certificate           *CertificateArchive `json:"certificate,omitempty"`
	Remediations          []RemediationItem   `json:"remediations,omitempty"`
	EnvironmentTrend      EnvironmentTrend    `json:"environment_trend,omitempty"`
	AuditInspection       *AuditInspection    `json:"audit_inspection,omitempty"`
}

type EnvironmentTrend struct {
	MaxTemperatureDeviation     float64                          `json:"max_temperature_deviation"`
	MaxHumidityDeviation        float64                          `json:"max_humidity_deviation"`
	AverageTemperatureDeviation float64                          `json:"average_temperature_deviation"`
	AverageHumidityDeviation    float64                          `json:"average_humidity_deviation"`
	ConsecutiveDriftCount       int                              `json:"consecutive_drift_count"`
	TriggerBatchIDs             []string                         `json:"trigger_batch_ids,omitempty"`
	Blocked                     bool                             `json:"blocked"`
	Resolved                    bool                             `json:"resolved"`
	Phases                      map[string]EnvironmentPhaseTrend `json:"phases,omitempty"`
}

type EnvironmentPhaseTrend struct {
	MaxTemperatureDeviation     float64 `json:"max_temperature_deviation"`
	MaxHumidityDeviation        float64 `json:"max_humidity_deviation"`
	AverageTemperatureDeviation float64 `json:"average_temperature_deviation"`
	AverageHumidityDeviation    float64 `json:"average_humidity_deviation"`
	ConsecutiveDriftCount       int     `json:"consecutive_drift_count"`
	BatchCount                  int     `json:"batch_count"`
}

type AuditInspection struct {
	Healthy       bool      `json:"healthy"`
	CheckedAt     time.Time `json:"checked_at"`
	BrokenEventID string    `json:"broken_event_id,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}
type Baseline struct {
	TemperatureC    float64   `json:"temperature_c"`
	HumidityPercent float64   `json:"humidity_percent"`
	GaugeConfig     string    `json:"gauge_config"`
	Fingerprint     string    `json:"fingerprint"`
	FrozenAt        time.Time `json:"frozen_at"`
	GaugeWarnings   []string  `json:"gauge_warnings,omitempty"`
}
type MeasurementBatch struct {
	BatchID              string         `json:"batch_id"`
	CaseID               string         `json:"case_id"`
	Phase                string         `json:"phase"`
	SampleDigest         string         `json:"sample_digest"`
	QualityState         string         `json:"quality_state"`
	AnomalyCode          string         `json:"anomaly_code,omitempty"`
	SampleCount          int            `json:"sample_count"`
	TemperatureC         float64        `json:"temperature_c"`
	HumidityPercent      float64        `json:"humidity_percent"`
	TemperatureDeviation float64        `json:"temperature_deviation,omitempty"`
	HumidityDeviation    float64        `json:"humidity_deviation,omitempty"`
	Mean                 float64        `json:"mean"`
	StdDev               float64        `json:"std_dev"`
	RecordedAt           time.Time      `json:"recorded_at"`
	Values               []float64      `json:"values"`
	Retest               bool           `json:"retest,omitempty"`
	Quality              QualityMetrics `json:"quality,omitempty"`
}
type QualityMetrics struct {
	Complete      bool    `json:"complete"`
	InRange       bool    `json:"in_range"`
	Repeatable    bool    `json:"repeatable"`
	EnvironmentOK bool    `json:"environment_ok"`
	Mean          float64 `json:"mean"`
	StdDev        float64 `json:"std_dev"`
	LowerBound    float64 `json:"lower_bound"`
	UpperBound    float64 `json:"upper_bound"`
	Threshold     string  `json:"threshold,omitempty"`
	Code          string  `json:"code"`
}
type Anomaly struct {
	AnomalyID   string          `json:"anomaly_id"`
	Code        string          `json:"code"`
	Description string          `json:"description"`
	Disposition string          `json:"disposition,omitempty"`
	BatchID     string          `json:"batch_id"`
	Resolved    bool            `json:"resolved"`
	CreatedAt   time.Time       `json:"created_at"`
	ResolvedAt  time.Time       `json:"resolved_at,omitempty"`
	Attempts    []RetestAttempt `json:"attempts,omitempty"`
}
type RetestAttempt struct {
	AttemptNo   int            `json:"attempt_no"`
	BatchID     string         `json:"batch_id"`
	QualityCode string         `json:"quality_code"`
	Quality     QualityMetrics `json:"quality"`
	Disposition string         `json:"disposition,omitempty"`
	Details     string         `json:"details,omitempty"`
	At          time.Time      `json:"at"`
}
type RemediationItem struct {
	ItemID           string    `json:"item_id"`
	Category         string    `json:"category"`
	Reason           string    `json:"reason"`
	Phase            string    `json:"phase,omitempty"`
	AnomalyID        string    `json:"anomaly_id,omitempty"`
	Status           string    `json:"status"`
	Explanation      string    `json:"explanation,omitempty"`
	EvidenceBatchIDs []string  `json:"evidence_batch_ids,omitempty"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	Escalated        bool      `json:"escalated,omitempty"`
}
type ReviewDecision struct {
	ReviewID        string    `json:"review_id"`
	CaseID          string    `json:"case_id"`
	ReviewerID      string    `json:"reviewer_id"`
	ReviewerRole    string    `json:"reviewer_role"`
	Decision        string    `json:"decision"`
	Rationale       string    `json:"rationale"`
	EvidenceDigest  string    `json:"evidence_digest"`
	SignedAt        time.Time `json:"signed_at"`
	RejectCategory  string    `json:"reject_category,omitempty"`
	AffectedPhase   string    `json:"affected_phase,omitempty"`
	AffectedAnomaly string    `json:"affected_anomaly,omitempty"`
}
type CertificateArchive struct {
	CertificateID      string    `json:"certificate_id"`
	CaseID             string    `json:"case_id"`
	CertificateDigest  string    `json:"certificate_digest"`
	IssuedBy           string    `json:"issued_by"`
	EvidenceBundlePath string    `json:"evidence_bundle_path"`
	AuditHead          string    `json:"audit_head"`
	IssuedAt           time.Time `json:"issued_at"`
	RetentionUntil     time.Time `json:"retention_until"`
}
type AuditEvent struct {
	ID             string            `json:"id"`
	CaseID         string            `json:"case_id"`
	Action         string            `json:"action"`
	Digest         string            `json:"digest"`
	PreviousDigest string            `json:"previous_digest"`
	At             time.Time         `json:"at"`
	Data           map[string]string `json:"data"`
}
