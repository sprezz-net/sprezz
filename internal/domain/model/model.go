package model

import "time"

type TermType int

const (
	NamedNode TermType = iota
	BlankNode
	Literal
)

type Quad struct {
	GraphID   int64
	Subject   string
	Predicate string
	Object    string
	ObjType   TermType
}

func (q Quad) IsLiteral() bool {
	return q.ObjType == Literal
}

type NomadicIdentity struct {
	GUID               string
	PrimaryHubURL      string
	MasterPublicKeyPEM string
	ClonedHubs         []string
}

type ObjectVersion struct {
	GraphID    int64
	ActivityID string
	ObjectIRI  string
	Payload    []byte
	CreatedAt  time.Time
}

type InboundTask struct {
	ID          string // UUIDv7
	ActivityIRI string
	ObjectIRI   string
	Payload     []byte
}
