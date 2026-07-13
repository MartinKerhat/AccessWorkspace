package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
		insert into audit_events (
			id, event_type, user_id, user_name, resource_id, resource_name, metadata
		) values ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, event.ID, event.EventType, event.UserID, event.UserName, event.ResourceID, event.ResourceName, payload)
	return err
}

// ListFilter narrows and pages the audit trail. Query and EventType filter in
// SQL so searches cover the full history, not just already-fetched pages.
type ListFilter struct {
	UserID    *string
	Query     string
	EventType string
	Limit     int
	Offset    int
}

func (f ListFilter) conditions() (string, []any) {
	clauses := []string{}
	args := []any{}
	if f.UserID != nil {
		args = append(args, *f.UserID)
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if f.EventType != "" {
		args = append(args, f.EventType)
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", len(args)))
	}
	if trimmed := strings.TrimSpace(f.Query); trimmed != "" {
		args = append(args, "%"+escapeLikePattern(trimmed)+"%")
		index := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(user_name ilike $%d or resource_name ilike $%d or resource_id ilike $%d or event_type ilike $%d)",
			index, index, index, index,
		))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " where " + strings.Join(clauses, " and "), args
}

func escapeLikePattern(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "%", `\%`)
	return strings.ReplaceAll(value, "_", `\_`)
}

// List returns one page of matching events plus the total match count.
func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Event, int, error) {
	where, args := filter.conditions()

	total := 0
	if err := r.db.QueryRow(ctx, "select count(*) from audit_events"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		select id, event_type, user_id, user_name, resource_id, resource_name, metadata, created_at
		from audit_events
	` + where
	query += fmt.Sprintf(" order by created_at desc limit $%d offset $%d", len(args)+1, len(args)+2)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.EventType, &event.UserID, &event.UserName, &event.ResourceID, &event.ResourceName, &metadata, &event.CreatedAt); err != nil {
			return nil, 0, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, 0, err
			}
		}
		if event.Metadata == nil {
			event.Metadata = map[string]any{}
		}
		events = append(events, event)
	}

	return events, total, rows.Err()
}

// ListEventTypes returns every event type present in the audit trail, so the
// UI filter offers the full set instead of only types on the loaded page.
func (r *Repository) ListEventTypes(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, "select distinct event_type from audit_events order by event_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := []string{}
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			return nil, err
		}
		types = append(types, eventType)
	}
	return types, rows.Err()
}
