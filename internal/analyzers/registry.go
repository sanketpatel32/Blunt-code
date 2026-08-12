package analyzers

import (
	"fmt"
	"sort"
)

type Registry struct {
	analyzers map[string]Analyzer
	order     []string
}

func NewRegistry() *Registry { return &Registry{analyzers: make(map[string]Analyzer)} }
func (r *Registry) Register(a Analyzer) error {
	if a == nil || a.ID() == "" {
		return fmt.Errorf("analyzer must have an id")
	}
	if _, found := r.analyzers[a.ID()]; found {
		return fmt.Errorf("analyzer %q already registered", a.ID())
	}
	r.analyzers[a.ID()] = a
	r.order = append(r.order, a.ID())
	return nil
}
func (r *Registry) Get(id string) (Analyzer, bool) { a, ok := r.analyzers[id]; return a, ok }
func (r *Registry) All() []Analyzer {
	out := make([]Analyzer, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.analyzers[id])
	}
	return out
}
func (r *Registry) IDs() []string {
	out := append([]string(nil), r.order...)
	sort.Strings(out)
	return out
}
