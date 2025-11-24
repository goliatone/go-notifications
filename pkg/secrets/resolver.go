package secrets

import "errors"

// SimpleResolver batches Get calls using a single provider.
type SimpleResolver struct {
	Provider Provider
}

// Resolve fetches each reference and returns a map keyed by the exact reference.
// ErrNotFound entries are skipped so callers can provide ordered fallbacks.
func (r SimpleResolver) Resolve(refs ...Reference) (map[Reference]SecretValue, error) {
	results := make(map[Reference]SecretValue, len(refs))
	if r.Provider == nil {
		return results, ErrUnsupported
	}
	for _, ref := range refs {
		val, err := r.Provider.Get(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		results[ref] = val
	}
	return results, nil
}
