package calibration

import "seismocal/internal/model"

func OpenAnomalies(c model.CalibrationCase) []model.Anomaly {
	out := make([]model.Anomaly, 0)
	for _, a := range c.Anomalies {
		if !a.Resolved {
			out = append(out, a)
		}
	}
	return out
}
func AllAnomaliesResolved(c model.CalibrationCase) bool { return len(OpenAnomalies(c)) == 0 }
