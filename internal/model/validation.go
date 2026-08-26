package model

import (
	"fmt"
	"regexp"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func ValidateIdentifier(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s不能为空", field)
	}
	if len(value) < 1 || len(value) > 64 || !identifier.MatchString(value) {
		return fmt.Errorf("%s格式无效", field)
	}
	return nil
}

func (c CalibrationCase) Validate() error {
	if err := ValidateIdentifier("case_id", c.CaseID); err != nil {
		return err
	}
	if err := ValidateIdentifier("station_code", c.StationCode); err != nil {
		return err
	}
	if err := ValidateIdentifier("serial_number", c.SerialNumber); err != nil {
		return err
	}
	if strings.TrimSpace(c.ResponsibleEngineer) == "" {
		return fmt.Errorf("responsible_engineer不能为空")
	}
	if c.Revision < 1 {
		return fmt.Errorf("revision无效")
	}
	return nil
}
func (b MeasurementBatch) HasIntegrity() bool {
	return b.SampleCount > 0 && b.SampleCount == len(b.Values) && b.SampleDigest != ""
}
func (r ReviewDecision) IsIndependent(c CalibrationCase) bool {
	return r.ReviewerID != "" && r.ReviewerID != c.ResponsibleEngineer && r.EvidenceDigest != ""
}
