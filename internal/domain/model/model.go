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

// QuadID is the lightweight, optimized integer variant designed for
// high-performance index traversal and batch database persistence pipelines.
type QuadID struct {
	GraphID     int64
	SubjectID   int64
	PredicateID int64
	ObjectID    int64
	IsLiteral   bool
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
