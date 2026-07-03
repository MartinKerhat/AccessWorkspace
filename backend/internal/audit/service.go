package audit

import (
	"context"

	"github.com/google/uuid"
)

type EventWriter interface {
	Create(ctx context.Context, event Event) error
	List(ctx context.Context, userID *string, limit int) ([]Event, error)
}

type Service struct {
	repo EventWriter
}

func NewService(repo EventWriter) *Service {
	return &Service{repo: repo}
}

func (s *Service) Log(ctx context.Context, params LogParams) error {
	event := Event{
		ID:           uuid.NewString(),
		EventType:    params.EventType,
		UserID:       params.UserID,
		UserName:     params.UserName,
		ResourceID:   params.ResourceID,
		ResourceName: params.ResourceName,
		Metadata:     params.Metadata,
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	return s.repo.Create(ctx, event)
}

func (s *Service) List(ctx context.Context, limit int) ([]Event, error) {
	return s.repo.List(ctx, nil, limit)
}

func (s *Service) RecentForUser(ctx context.Context, userID string, limit int) ([]Event, error) {
	return s.repo.List(ctx, &userID, limit)
}

type LogParams struct {
	EventType    EventType
	UserID       string
	UserName     string
	ResourceID   *string
	ResourceName *string
	Metadata     map[string]any
}
