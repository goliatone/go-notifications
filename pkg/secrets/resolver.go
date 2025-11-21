package secrets

// SimpleResolver batches Get calls using a single provider.
type SimpleResolver struct {
	Provider Provider
}

// Resolve fetches each reference and returns a map keyed by the exact reference.
func (r SimpleResolver) Resolve(refs ...Reference) (map[Reference]SecretValue, error) {
	results := make(map[Reference]SecretValue, len(refs))
	if r.Provider == nil {
		return results, ErrUnsupported
	}
	for _, ref := range refs {
		val, err := r.Provider.Get(ref)
		if err != nil {
			return nil, err
		}
		results[ref] = val
	}
	return results, nil
}
