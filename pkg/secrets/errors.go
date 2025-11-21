package secrets

import (
	"errors"
	"strings"
)

var (
	ErrNotFound      = errors.New("secrets: not found")
	ErrUnauthorized  = errors.New("secrets: unauthorized")
	ErrInvalidScope  = errors.New("secrets: invalid scope")
	ErrInvalidRef    = errors.New("secrets: invalid reference")
	ErrUnsupported   = errors.New("secrets: unsupported operation")
	ErrEmptyValue    = errors.New("secrets: empty value")
	ErrInvalidKey    = errors.New("secrets: invalid key")
	ErrInvalidTarget = errors.New("secrets: invalid target")
)

// ValidateReference performs basic checks on a reference.
func ValidateReference(ref Reference) error {
	if ref.Scope == "" || !isValidScope(ref.Scope) {
		return ErrInvalidScope
	}
	if strings.TrimSpace(ref.SubjectID) == "" {
		return ErrInvalidRef
	}
	if strings.TrimSpace(ref.Channel) == "" || strings.TrimSpace(ref.Provider) == "" || strings.TrimSpace(ref.Key) == "" {
		return ErrInvalidKey
	}
	return nil
}

func isValidScope(scope Scope) bool {
	switch scope {
	case ScopeSystem, ScopeTenant, ScopeUser:
		return true
	default:
		return false
	}
}
