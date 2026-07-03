package resources

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type launchTicketRecord struct {
	payload   LaunchPayload
	expiresAt time.Time
}

type launchTicketStore struct {
	mu      sync.Mutex
	records map[string]launchTicketRecord
}

func newLaunchTicketStore() *launchTicketStore {
	return &launchTicketStore{
		records: map[string]launchTicketRecord{},
	}
}

func (s *launchTicketStore) Issue(payload LaunchPayload, ttl time.Duration) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	sweepBefore := time.Now().UTC()
	for key, record := range s.records {
		if !record.expiresAt.After(sweepBefore) {
			delete(s.records, key)
		}
	}

	id := uuid.NewString()
	s.records[id] = launchTicketRecord{
		payload:   payload,
		expiresAt: sweepBefore.Add(ttl),
	}
	return id
}

func (s *launchTicketStore) Redeem(ticket string) (LaunchPayload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ticket = strings.TrimSpace(ticket)
	record, ok := s.records[ticket]
	if !ok {
		return LaunchPayload{}, ErrNotFound
	}
	delete(s.records, ticket)
	if !record.expiresAt.After(time.Now().UTC()) {
		return LaunchPayload{}, ErrNotFound
	}
	return record.payload, nil
}
