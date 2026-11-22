package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// idCounter makes ID generation infallible: row IDs need uniqueness, not
// cryptographic strength, so a crypto/rand outage must not crash a request
// (previously panic()ed mid-insert). The counter + time seed keeps collisions
// impossible within a process even if rand fails, and the rand suffix keeps
// cross-process collisions improbable.
var idCounter atomic.Uint64

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// No panic in library write paths: fall back to a time/counter ID. A
		// crypto/rand failure is surfaced by NewToken (which cannot fall back),
		// not by database row identifiers.
		seq := idCounter.Add(1)
		return fmt.Sprintf("%012x-%04x-%04x-%04x-%012x", time.Now().UnixNano(), seq&0xffff, (seq>>16)&0xffff, os.Getpid()&0xffff, time.Now().UnixMicro()&0xffffffffffff)
	}
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:])
}

func dbTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
