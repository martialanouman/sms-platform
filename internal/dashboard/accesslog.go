package dashboard

import (
	"context"
	"time"

	"github.com/martialanouman/sms-platform/internal/platform/keyset"
)

// AccessLogFilter narrows the access log to an actor and a period, To excluded.
type AccessLogFilter struct {
	Actor *string
	From  *time.Time
	To    *time.Time
}

// AccessLog is a page of the access log under f, newest first, and the cursor of the next one, empty on the last page.
func (s Service) AccessLog(ctx context.Context, f AccessLogFilter, cursor string) ([]Entry, string, error) {
	var before *Entry
	if cursor != "" {
		at, id, ok := keyset.Parse(cursor)
		if !ok {
			return nil, "", ErrBadCursor
		}
		before = &Entry{At: at, ID: id}
	}
	var page []Entry
	err := s.Store.Tx(ctx, func(tx Tx) (err error) {
		page, err = tx.AccessLog(ctx, f, before, accessLogPage+1)
		return err
	})
	if err != nil || len(page) <= accessLogPage {
		return page, "", err
	}
	page = page[:accessLogPage]
	last := page[accessLogPage-1]
	return page, keyset.Encode(last.At, last.ID), nil
}
