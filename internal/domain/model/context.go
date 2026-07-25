package model

// ContextKey defines a unique, non-collision type for context operations.
type ContextKey int

const (
	// TenantIDKey uniquely identifies the multi-tenant identifier inside execution flows.
	TenantIDKey ContextKey = iota
	// ActorIRIKey uniquely identifies the authenticated actor string inside execution flows.
	ActorIRIKey
)
