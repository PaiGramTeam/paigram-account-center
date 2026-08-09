package httpserver

import "sync"

// Access describes the authentication boundary of an HTTP operation.
type Access struct {
	Public        bool
	Authenticated bool
	Permission    string
}

// Route is the stable catalog entry emitted for a registered operation.
type Route struct {
	OperationID string
	Method      string
	Path        string
	Access      Access
}

// Catalog records every business operation registered through Huma.
type Catalog struct {
	mu     sync.RWMutex
	routes []Route
	keys   map[string]struct{}
}

func newCatalog() *Catalog {
	return &Catalog{keys: make(map[string]struct{})}
}

func (c *Catalog) add(route Route) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := route.Method + " " + route.Path
	if _, exists := c.keys[key]; exists {
		panic("duplicate HTTP operation: " + key)
	}
	c.keys[key] = struct{}{}
	c.routes = append(c.routes, route)
}

// Routes returns a registration-order snapshot of the route catalog.
func (c *Catalog) Routes() []Route {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Route(nil), c.routes...)
}
