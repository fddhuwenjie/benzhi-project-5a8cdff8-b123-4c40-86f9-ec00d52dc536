package calibration

import (
	"fmt"
	"math"
	"seismocal/internal/model"
	"sort"
	"time"
)

type PhaseSummary struct {
	Phase       string    `json:"phase"`
	BatchCount  int       `json:"batch_count"`
	SampleCount int       `json:"sample_count"`
	MeanMin     float64   `json:"mean_min"`
	MeanMax     float64   `json:"mean_max"`
	StdDevMin   float64   `json:"std_dev_min"`
	StdDevMax   float64   `json:"std_dev_max"`
	PassRate    float64   `json:"pass_rate"`
	LatestAt    time.Time `json:"latest_at"`
}
type MeasurementSummary struct {
	Revision    int            `json:"revision"`
	Phases      []PhaseSummary `json:"phases"`
	Diagnostics []string       `json:"diagnostics"`
	From        time.Time      `json:"from,omitempty"`
	To          time.Time      `json:"to,omitempty"`
}

func Summarize(c model.CalibrationCase, phase string, from, to time.Time) MeasurementSummary {
	groups := map[string][]model.MeasurementBatch{}
	for _, b := range c.Batches {
		p := normalizePhase(b.Phase)
		if phase != "" && p != phase {
			continue
		}
		if !from.IsZero() && b.RecordedAt.Before(from) {
			continue
		}
		if !to.IsZero() && b.RecordedAt.After(to) {
			continue
		}
		groups[p] = append(groups[p], b)
	}
	out := MeasurementSummary{Revision: c.Revision, From: from, To: to}
	for _, p := range phaseOrder {
		bs := groups[p]
		if len(bs) == 0 {
			continue
		}
		ps := PhaseSummary{Phase: p, BatchCount: len(bs), MeanMin: math.Inf(1), StdDevMin: math.Inf(1)}
		pass := 0
		for _, b := range bs {
			ps.SampleCount += b.SampleCount
			if b.Mean < ps.MeanMin {
				ps.MeanMin = b.Mean
			}
			if b.Mean > ps.MeanMax {
				ps.MeanMax = b.Mean
			}
			if b.StdDev < ps.StdDevMin {
				ps.StdDevMin = b.StdDev
			}
			if b.StdDev > ps.StdDevMax {
				ps.StdDevMax = b.StdDev
			}
			if b.QualityState == "passed" {
				pass++
			}
			if b.RecordedAt.After(ps.LatestAt) {
				ps.LatestAt = b.RecordedAt
			}
		}
		ps.PassRate = float64(pass) / float64(len(bs))
		out.Phases = append(out.Phases, ps)
	}
	// Independent diagnostics make gaps and repeated/failed phases obvious.
	for _, p := range phaseOrder {
		if len(groups[p]) == 0 {
			out.Diagnostics = append(out.Diagnostics, fmt.Sprintf("缺少阶段: %s", p))
			continue
		}
		pass := false
		for _, b := range groups[p] {
			if b.QualityState == "passed" {
				pass = true
			}
		}
		if !pass {
			out.Diagnostics = append(out.Diagnostics, fmt.Sprintf("阶段仅有失败批次: %s", p))
		}
	}
	seen := map[string]int{}
	for _, b := range c.Batches {
		if b.Retest {
			continue
		}
		seen[normalizePhase(b.Phase)]++
	}
	for p, n := range seen {
		if n > 1 {
			out.Diagnostics = append(out.Diagnostics, fmt.Sprintf("重复阶段: %s", p))
		}
	}
	sort.Strings(out.Diagnostics)
	return out
}
