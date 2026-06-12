package capability

import (
	"fmt"
	"sync"
)

// Registry stores capability descriptors by executable name.
// Registry 按可执行名称存储 capability descriptor。
type Registry struct {
	mu         sync.RWMutex
	descriptor map[string]Descriptor
}

// NewRegistry creates a registry from descriptors.
// NewRegistry 基于 descriptor 列表创建 registry。
func NewRegistry(descriptors ...Descriptor) (*Registry, error) {
	registry := &Registry{descriptor: make(map[string]Descriptor, len(descriptors))}
	for _, descriptor := range descriptors {
		if err := registry.Register(descriptor); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register adds one descriptor to the registry.
// Register 向 registry 添加一个 descriptor。
func (r *Registry) Register(descriptor Descriptor) error {
	if r == nil {
		return fmt.Errorf("capability registry is nil")
	}
	if err := descriptor.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.descriptor[descriptor.Name]; exists {
		return fmt.Errorf("capability already registered: %s", descriptor.Name)
	}
	r.descriptor[descriptor.Name] = descriptor
	return nil
}

// Get returns the descriptor for name.
// Get 返回指定名称对应的 descriptor。
func (r *Registry) Get(name string) (Descriptor, bool) {
	if r == nil {
		return Descriptor{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	descriptor, ok := r.descriptor[name]
	return descriptor, ok
}
