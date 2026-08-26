package calibration

import (
	"math"
	"seismocal/internal/model"
)

type QualityReport struct {
	Complete, InRange, Repeatable, EnvironmentOK bool
	Mean, StdDev                                 float64
	Code                                         string
}

func Evaluate(values []float64) QualityReport {
	r := QualityReport{Complete: len(values) > 0, EnvironmentOK: true}
	if !r.Complete {
		r.Code = "EMPTY_SAMPLE"
		return r
	}
	min, max, sum := values[0], values[0], 0.0
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			r.Code = "NON_FINITE"
			return r
		}
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}
	r.Mean = sum / float64(len(values))
	r.InRange = min >= -100 && max <= 100
	v := 0.0
	for _, x := range values {
		v += (x - r.Mean) * (x - r.Mean)
	}
	r.StdDev = math.Sqrt(v / float64(len(values)))
	r.Repeatable = r.StdDev <= 5
	if !r.InRange {
		r.Code = "RANGE_OUT_OF_BOUNDS"
	} else if !r.Repeatable {
		r.Code = "REPEATABILITY_HIGH"
	} else {
		r.Code = "PASS"
	}
	return r
}

func (r QualityReport) Metrics() model.QualityMetrics {
	return model.QualityMetrics{Complete: r.Complete, InRange: r.InRange, Repeatable: r.Repeatable, EnvironmentOK: r.EnvironmentOK, Mean: r.Mean, StdDev: r.StdDev, LowerBound: -100, UpperBound: 100, Threshold: "range[-100,100], std_dev<=5", Code: r.Code}
}
