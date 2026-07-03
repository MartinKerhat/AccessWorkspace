package audit

import "time"

type EventType string

const (
	EventResourceViewed    EventType = "resource_viewed"
	EventResourceRevealed  EventType = "resource_revealed"
	EventResourceFilled    EventType = "resource_filled"
	EventResourceLaunched  EventType = "resource_launched"
	EventResourceCreated   EventType = "resource_created"
	EventResourceUpdated   EventType = "resource_updated"
	EventResourceArchived  EventType = "resource_archived"
	EventResourceRestored  EventType = "resource_restored"
	EventUserAccessUpdated EventType = "user_access_updated"
)

type Event struct {
	ID           string         `json:"id"`
	EventType    EventType      `json:"eventType"`
	UserID       string         `json:"userId"`
	UserName     string         `json:"userName"`
	ResourceID   *string        `json:"resourceId,omitempty"`
	ResourceName *string        `json:"resourceName,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}
