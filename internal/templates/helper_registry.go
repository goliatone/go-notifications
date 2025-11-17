package templates

import (
	"sync"

	gotemplate "github.com/goliatone/go-template"
)

// helperRegistry keeps helper functions in sync with the go-template renderer.
type helperRegistry struct {
	mu       sync.RWMutex
	funcs    map[string]any
	renderer *gotemplate.Engine
}

func newHelperRegistry(renderer *gotemplate.Engine) *helperRegistry {
	return &helperRegistry{
		funcs:    make(map[string]any),
		renderer: renderer,
	}
}

func (r *helperRegistry) Register(funcs map[string]any) {
	if r == nil || len(funcs) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, fn := range funcs {
		if fn == nil {
			delete(r.funcs, key)
			continue
		}
		r.funcs[key] = fn
	}
	gotemplate.WithTemplateFunc(funcs)(r.renderer)
}

func (r *helperRegistry) Funcs() map[string]any {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]any, len(r.funcs))
	for k, v := range r.funcs {
		out[k] = v
	}
	return out
}
