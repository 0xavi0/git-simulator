// Package provider holds the global registry of git-vendor Provider implementations.
// Each vendor package registers itself in its init() function.
package provider

import (
	"fmt"
	"sync"

	"github.com/rancher/gitsim/pkg/core"
)

var (
	mu       sync.RWMutex
	registry = map[string]core.Provider{}
)

// Register adds p to the global registry. Conventionally called from
// vendor package init() functions. Safe for concurrent use.
func Register(p core.Provider) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.Name()] = p
}

// Get returns the named provider or an error if it is not registered.
func Get(name string) (core.Provider, error) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", name)
	}
	return p, nil
}
