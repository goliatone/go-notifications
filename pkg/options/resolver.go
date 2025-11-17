package options

import (
	"errors"
	"fmt"

	opts "github.com/goliatone/go-options"
	layering "github.com/goliatone/go-options/layering"
)

// Snapshot captures the immutable payload associated with a scope layer.
type Snapshot struct {
	Scope      opts.Scope
	Data       map[string]any
	SnapshotID string
}

// Resolver wraps a go-options Options value exposing typed helpers.
type Resolver struct {
	options *opts.Options[map[string]any]
}

var (
	// ErrNoSnapshots signals that at least one scope snapshot must be provided.
	ErrNoSnapshots = errors.New("options: at least one snapshot is required")
)

// NewResolver merges the provided scope snapshots ordered by their scope
// priority and returns a resolver exposing trace + schema helpers.
func NewResolver(snapshots ...Snapshot) (*Resolver, error) {
	if len(snapshots) == 0 {
		return nil, ErrNoSnapshots
	}

	layers := make([]opts.Layer[map[string]any], 0, len(snapshots))
	for _, snap := range snapshots {
		if snap.Scope.Name == "" {
			return nil, fmt.Errorf("options: snapshot scope name is required")
		}
		layerOpts := []opts.LayerOption[map[string]any]{}
		if snap.SnapshotID != "" {
			layerOpts = append(layerOpts, opts.WithSnapshotID[map[string]any](snap.SnapshotID))
		}
		payload := cloneMap(snap.Data)
		layers = append(layers, opts.NewLayer(snap.Scope, payload, layerOpts...))
	}

	stack, err := opts.NewStack(layers...)
	if err != nil {
		return nil, err
	}
	merged, err := stack.Merge()
	if err != nil {
		return nil, err
	}
	return &Resolver{options: merged}, nil
}

// Options returns a cloned copy of the underlying go-options wrapper so callers
// can interact with low-level helpers directly.
func (r *Resolver) Options() *opts.Options[map[string]any] {
	if r == nil || r.options == nil {
		return nil
	}
	return r.options.Clone()
}

// Resolve fetches the value stored at path and returns the accompanying trace.
func (r *Resolver) Resolve(path string) (any, opts.Trace, error) {
	if r == nil || r.options == nil {
		return nil, opts.Trace{Path: path}, fmt.Errorf("options: resolver not initialised")
	}
	return r.options.ResolveWithTrace(path)
}

// ResolveBool resolves the value at path and ensures it is a boolean.
func (r *Resolver) ResolveBool(path string) (bool, opts.Trace, error) {
	value, trace, err := r.Resolve(path)
	if err != nil {
		return false, trace, err
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, trace, fmt.Errorf("options: path %s is not a boolean", path)
	}
	return boolean, trace, nil
}

// ResolveString resolves the value at path and ensures it is a string.
func (r *Resolver) ResolveString(path string) (string, opts.Trace, error) {
	value, trace, err := r.Resolve(path)
	if err != nil {
		return "", trace, err
	}
	str, ok := value.(string)
	if !ok {
		return "", trace, fmt.Errorf("options: path %s is not a string", path)
	}
	return str, trace, nil
}

// ResolveStringSlice resolves the value at path and converts it into []string.
func (r *Resolver) ResolveStringSlice(path string) ([]string, opts.Trace, error) {
	value, trace, err := r.Resolve(path)
	if err != nil {
		return nil, trace, err
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...), trace, nil
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, trace, fmt.Errorf("options: path %s contains non-string entries", path)
			}
			out[i] = str
		}
		return out, trace, nil
	default:
		return nil, trace, fmt.Errorf("options: path %s is not a string slice", path)
	}
}

// Schema exports the schema associated with the resolver's options snapshot.
func (r *Resolver) Schema() (opts.SchemaDocument, error) {
	if r == nil || r.options == nil {
		return opts.SchemaDocument{}, fmt.Errorf("options: resolver not initialised")
	}
	return r.options.Schema()
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	return layering.Clone(src)
}
