package audit

import (
	"context"
	"encoding/json"
	"fmt"

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

func (r *Repository) List(ctx context.Context, userID *string, limit int) ([]Event, error) {
	query := `
		select id, event_type, user_id, user_name, resource_id, resource_name, metadata, created_at
		from audit_events
	`
	args := []any{}
	if userID != nil {
		query += ` where user_id = $1`
		args = append(args, *userID)
	}
	query += fmt.Sprintf(" order by created_at desc limit $%d", len(args)+1)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var metadata []byte
		if err := rows.Scan(&event.ID, &event.EventType, &event.UserID, &event.UserName, &event.ResourceID, &event.ResourceName, &metadata, &event.CreatedAt); err != nil {
			return nil, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, err
			}
		}
		if event.Metadata == nil {
			event.Metadata = map[string]any{}
		}
		events = append(events, event)
	}

	return events, rows.Err()
}
