package timeline_filter_cache_pollution

import (
	"testing"
	"time"

	"seismocal/internal/storage"
)

func TestFilteredTimelineDoesNotPolluteLaterReads(t *testing.T) {
	st := storage.New(t.TempDir())
	const caseID = "CASE-TIMELINE-CACHE"
	st.AppendAudit(caseID, "case_created", nil)
	st.AppendAudit(caseID, "baseline_frozen", nil)

	filtered := st.Timeline(caseID, "case_created", time.Time{}, time.Time{})
	if len(filtered) != 1 || filtered[0].Action != "case_created" {
		t.Fatalf("filtered timeline = %#v", filtered)
	}

	all := st.Timeline(caseID, "", time.Time{}, time.Time{})
	if len(all) != 2 {
		t.Fatalf("unfiltered timeline polluted by cached filter: length = %d, want 2", len(all))
	}
}
