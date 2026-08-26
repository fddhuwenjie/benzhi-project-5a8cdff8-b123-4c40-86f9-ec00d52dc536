package review

import (
	"seismocal/internal/calibration"
	"seismocal/internal/model"
)

func EligibleForReview(c model.CalibrationCase) bool {
	if c.Status != model.StatusMeasuring || len(c.Batches) == 0 || !calibration.AllAnomaliesResolved(c) || !calibration.MeasurementComplete(c) {
		return false
	}
	for _, b := range c.Batches {
		if !BatchAccepted(c, b) {
			return false
		}
	}
	return true
}

func BatchAccepted(c model.CalibrationCase, b model.MeasurementBatch) bool {
	if b.QualityState == "passed" {
		return true
	}
	for _, a := range c.Anomalies {
		if a.BatchID == b.BatchID {
			return a.Resolved
		}
	}
	return false
}
