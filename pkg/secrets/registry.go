package secrets

// Registry can resolve a provider by scope.
type Registry struct {
	System Provider
	Tenant Provider
	User   Provider
}

// ProviderFor returns the provider matching the scope, or nil.
func (r Registry) ProviderFor(scope Scope) Provider {
	switch scope {
	case ScopeSystem:
		return r.System
	case ScopeTenant:
		return r.Tenant
	case ScopeUser:
		return r.User
	default:
		return nil
	}
}

// Resolve dispatches to the provider for each reference.
func (r Registry) Resolve(refs ...Reference) (map[Reference]SecretValue, error) {
	results := make(map[Reference]SecretValue, len(refs))
	for _, ref := range refs {
		prov := r.ProviderFor(ref.Scope)
		if prov == nil {
			return nil, ErrNotFound
		}
		val, err := prov.Get(ref)
		if err != nil {
			return nil, err
		}
		results[ref] = val
	}
	return results, nil
}
