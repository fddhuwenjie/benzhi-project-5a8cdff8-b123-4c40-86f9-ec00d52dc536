package case_service

import (
	"fmt"
	"seismocal/internal/model"
)

func CanAccept(status model.CaseStatus, command string) bool {
	switch command {
	case "freeze":
		return status == model.StatusDraft
	case "measure", "pause":
		return status == model.StatusMeasuring
	case "retest":
		return status == model.StatusPaused
	case "review":
		return status == model.StatusMeasuring
	case "issue":
		return status == model.StatusReview
	}
	return false
}
func RequireRevision(actual, expected int) error {
	if actual != expected {
		return fmt.Errorf("revision冲突: expected=%d actual=%d", expected, actual)
	}
	return nil
}
