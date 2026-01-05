package links

import "context"

// NopStore implements LinkStore without persistence.
type NopStore struct{}

var _ LinkStore = (*NopStore)(nil)

// Save discards records and returns nil.
func (n *NopStore) Save(ctx context.Context, records []LinkRecord) error {
	return nil
}

// NopObserver implements LinkObserver without side effects.
type NopObserver struct{}

var _ LinkObserver = (*NopObserver)(nil)

// OnLinksResolved ignores the link resolution event.
func (n *NopObserver) OnLinksResolved(ctx context.Context, info LinkResolution) {}
