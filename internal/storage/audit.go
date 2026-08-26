package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"seismocal/internal/model"
	"time"
)

func VerifyAuditChain(events []model.AuditEvent) bool {
	prev := ""
	for _, e := range events {
		raw, _ := json.Marshal(e.Data)
		sum := sha256.Sum256(append([]byte(prev+e.Action+e.At.Format(time.RFC3339Nano)), raw...))
		if e.PreviousDigest != prev || e.Digest != hex.EncodeToString(sum[:]) {
			return false
		}
		prev = e.Digest
	}
	return true
}
