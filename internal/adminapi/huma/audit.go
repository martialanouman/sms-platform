package huma

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/martialanouman/sms-platform/internal/adminapi"
)

type auditEntry struct {
	ID          string    `json:"id"`
	Operator    string    `json:"operator"`
	OperationID string    `json:"operation_id"`
	Target      string    `json:"target"`
	RequestID   *string   `json:"request_id"`
	Status      int       `json:"status"`
	Outcome     *string   `json:"outcome"`
	At          time.Time `json:"at"`
}

type auditChange struct {
	Actor  string          `json:"actor"`
	Table  string          `json:"table"`
	PK     string          `json:"pk"`
	Op     string          `json:"op"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
	At     time.Time       `json:"at"`
}

type pageOutput[T any] struct {
	Body struct {
		Data       []T     `json:"data"`
		NextCursor *string `json:"next_cursor"`
	}
}

type countedPageOutput[T any] struct {
	Body struct {
		Data       []T     `json:"data"`
		NextCursor *string `json:"next_cursor"`
		Total      int     `json:"total" minimum:"0"`
	}
}

func counted[T any](data []T, next string, total int) *countedPageOutput[T] {
	out := &countedPageOutput[T]{}
	out.Body.Data = data
	if next != "" {
		out.Body.NextCursor = &next
	}
	out.Body.Total = total
	return out
}

func page[T any](data []T, next string) *pageOutput[T] {
	out := &pageOutput[T]{}
	out.Body.Data = data
	if next != "" {
		out.Body.NextCursor = &next
	}
	return out
}

type listAuditLogInput struct {
	Operator string `query:"operator" maxLength:"200"`
	FromDate string `query:"from_date" format:"date-time"`
	ToDate   string `query:"to_date" format:"date-time"`
	Cursor   string `query:"cursor" maxLength:"1024"`
	Limit    int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

type listAuditChangesInput struct {
	RequestID string `path:"request_id" minLength:"1" maxLength:"200"`
	Cursor    string `query:"cursor" maxLength:"1024"`
	Limit     int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}

func security(scope string) []map[string][]string {
	return []map[string][]string{{scheme: {scope}}}
}

func registerAudit(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "list-audit-log", Method: http.MethodGet, Path: "/admin/audit-log",
		Tags: []string{"audit"}, Security: security("audit:read"), Errors: []int{401, 403, 422}},
		func(ctx context.Context, in *listAuditLogInput) (*pageOutput[auditEntry], error) {
			f := adminapi.Filter{From: instant(in.FromDate), To: instant(in.ToDate)}
			if in.Operator != "" {
				f.Operator = &in.Operator
			}
			entries, next, err := adminapi.AuditLogPage(ctx, adminapi.TxFrom(ctx), f, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]auditEntry, len(entries))
			for i, e := range entries {
				out[i] = auditEntry{ID: e.ID, Operator: e.Operator, OperationID: e.OperationID, Target: e.Target,
					RequestID: e.RequestID, Status: e.Status, Outcome: e.Outcome, At: e.At}
			}
			return page(out, next), nil
		})
	huma.Register(api, huma.Operation{OperationID: "list-audit-changes", Method: http.MethodGet,
		Path: "/admin/audit-log/{request_id}/changes", Tags: []string{"audit"}, Security: security("audit:read"),
		Errors: []int{401, 403, 404, 422}},
		func(ctx context.Context, in *listAuditChangesInput) (*pageOutput[auditChange], error) {
			changes, next, err := adminapi.ChangesPage(ctx, adminapi.TxFrom(ctx), in.RequestID, in.Cursor, in.Limit)
			if err != nil {
				return nil, err
			}
			out := make([]auditChange, len(changes))
			for i, c := range changes {
				out[i] = auditChange{Actor: c.Actor, Table: c.Table, PK: c.PK, Op: c.Op, Before: c.Before, After: c.After, At: c.At}
			}
			return page(out, next), nil
		})
}

func instant(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
