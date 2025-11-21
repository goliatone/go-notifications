package secrets

// NopProvider always returns ErrNotFound. Useful when provider is optional.
type NopProvider struct{}

func (NopProvider) Get(ref Reference) (SecretValue, error) { return SecretValue{}, ErrNotFound }
func (NopProvider) Put(ref Reference, value []byte) (string, error) {
	return "", ErrUnsupported
}
func (NopProvider) Delete(ref Reference) error                     { return ErrUnsupported }
func (NopProvider) Describe(ref Reference) (map[string]any, error) { return nil, ErrNotFound }
