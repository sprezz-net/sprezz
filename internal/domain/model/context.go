package model

import "time"

// ContextKey defines a unique, non-collision type for context operations.
type ContextKey int

const (
	// TenantIDKey uniquely identifies the multi-tenant identifier inside execution flows.
	TenantIDKey ContextKey = iota
	// ActorIRIKey uniquely identifies the authenticated actor string inside execution flows.
	ActorIRIKey
	// CollectionSyncHeaderKey uniquely identifies the collection-synchronization context inside execution flows.
	CollectionSyncHeaderKey
)

type detachedContext struct {
	parent interface {
		Value(key any) any
	}
}

func (d detachedContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (d detachedContext) Done() <-chan struct{}       { return nil }
func (d detachedContext) Err() error                  { return nil }
func (d detachedContext) Value(key any) any           { return d.parent.Value(key) }

// Detach returns a context that carries the parent values but ignores cancellation and deadlines.
func Detach(ctx interface {
	Value(key any) any
}) interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
} {
	return detachedContext{parent: ctx}
}
