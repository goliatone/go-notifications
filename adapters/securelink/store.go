package securelink

import (
	"context"
	"sync"

	"github.com/goliatone/go-notifications/pkg/links"
)

// MemoryStore keeps link records in memory for demos and tests.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]links.LinkRecord
}

var _ links.LinkStore = (*MemoryStore)(nil)

// NewMemoryStore creates an in-memory LinkStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string]links.LinkRecord),
	}
}

// Save stores records by URL (idempotent).
func (s *MemoryStore) Save(ctx context.Context, records []links.LinkRecord) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.records == nil {
		s.records = make(map[string]links.LinkRecord)
	}
	for _, record := range records {
		if record.URL == "" {
			continue
		}
		record.Metadata = cloneMetadata(record.Metadata)
		s.records[record.URL] = record
	}
	return nil
}

// Records returns a snapshot of stored records.
func (s *MemoryStore) Records() []links.LinkRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]links.LinkRecord, 0, len(s.records))
	for _, record := range s.records {
		record.Metadata = cloneMetadata(record.Metadata)
		out = append(out, record)
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
