// Package keyset is the opaque cursor of a page ordered by time and UUID, newest first.
package keyset

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Encode is the cursor after the row at, id.
func Encode(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(at.UnixMicro(), 10) + " " + id))
}

// Parse is the row a cursor of Encode points after.
func Parse(cursor string) (time.Time, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", false
	}
	at, id, _ := strings.Cut(string(raw), " ")
	micros, err := strconv.ParseInt(at, 10, 64)
	if err != nil || !uuid.MatchString(id) {
		return time.Time{}, "", false
	}
	return time.UnixMicro(micros), id, true
}
