package dashboard

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// AccessLog is a page of the access log, newest first, and the cursor of the next one, empty on the last page.
func (s Service) AccessLog(ctx context.Context, cursor string) ([]Entry, string, error) {
	var before *Entry
	if cursor != "" {
		e, ok := parseCursor(cursor)
		if !ok {
			return nil, "", ErrBadCursor
		}
		before = &e
	}
	var page []Entry
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		page, err = tx.AccessLog(ctx, before, accessLogPage+1)
		return err
	})
	if err != nil || len(page) <= accessLogPage {
		return page, "", err
	}
	page = page[:accessLogPage]
	last := page[accessLogPage-1]
	return page, base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(last.At.UnixMicro(), 10) + " " + last.ID)), nil
}

func parseCursor(cursor string) (Entry, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return Entry{}, false
	}
	at, id, found := strings.Cut(string(raw), " ")
	micros, err := strconv.ParseInt(at, 10, 64)
	if !found || err != nil || len(id) != 36 {
		return Entry{}, false
	}
	return Entry{At: time.UnixMicro(micros), ID: id}, true
}
