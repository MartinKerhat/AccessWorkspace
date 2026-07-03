package audit

import (
	"context"
	"testing"
)

type stubRepo struct {
	events []Event
}

func (s *stubRepo) Create(_ context.Context, event Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubRepo) List(_ context.Context, _ *string, _ int) ([]Event, error) {
	return s.events, nil
}

func TestServiceLogStoresMetadata(t *testing.T) {
	repo := &stubRepo{}
	service := NewService(repo)

	resourceID := "res-1"
	resourceName := "Bastion"

	err := service.Log(context.Background(), LogParams{
		EventType:    EventResourceLaunched,
		UserID:       "alice",
		UserName:     "Alice Admin",
		ResourceID:   &resourceID,
		ResourceName: &resourceName,
		Metadata: map[string]any{
			"type": "ssh",
		},
	})
	if err != nil {
		t.Fatalf("log event: %v", err)
	}

	if len(repo.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(repo.events))
	}

	if repo.events[0].Metadata["type"] != "ssh" {
		t.Fatalf("expected metadata to contain type")
	}
}
