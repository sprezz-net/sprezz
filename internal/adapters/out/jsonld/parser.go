package jsonld

import (
	"context"
	"fmt"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"

	"github.com/piprate/json-gold/ld"
)

// Defined package-level constants to centralize XML schema and shorthand vocab namespaces,
// resolving all duplicated string literal metrics across the adapter methods.
const (
	w3cXMLSchemaNS   = "http://www.w3.org/2001/XMLSchema"
	w3cDateTimeType  = "http://www.w3.org/2001/XMLSchema#dateTime"
	shortHTTPW3C     = "http://w3.org"
	shortHTTPSW3C    = "https://w3.org"
)

type JSONLDParser struct {
	loader   ld.DocumentLoader
	rewriter *BNodeRewriter
}

func NewJSONLDParser() *JSONLDParser {
	return &JSONLDParser{
		loader:   NewEmbeddedDocumentLoader(),
		rewriter: NewBNodeRewriter(),
	}
}

var _ ports.JSONLDParserPort = (*JSONLDParser)(nil)

func (p *JSONLDParser) ToQuads(ctx context.Context, graphID int64, mainObjectIRI string, jsonPayload []byte) ([]model.Quad, error) {
	proc := ld.NewJsonLdProcessor()
	options := ld.NewJsonLdOptions("")
	options.DocumentLoader = p.loader

	doc, err := ld.DocumentFromReader(strings.NewReader(string(jsonPayload)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON-LD document: %w", err)
	}

	rdfDataset, err := proc.ToRDF(doc, options)
	if err != nil {
		return nil, fmt.Errorf("failed to convert JSON-LD to RDF dataset: %w", err)
	}

	dataset, ok := rdfDataset.(*ld.RDFDataset)
	if !ok {
		return nil, fmt.Errorf("unexpected RDF dataset type: %T", rdfDataset)
	}

	var rawQuads []model.Quad
	for _, quad := range dataset.Graphs["@default"] {
		if quad == nil {
			continue
		}

		objVal, termType := evaluateNode(quad.Object, quad.Predicate.GetValue())

		rawQuads = append(rawQuads, model.Quad{
			GraphID:   graphID,
			Subject:   quad.Subject.GetValue(),
			Predicate: quad.Predicate.GetValue(),
			Object:    objVal,
			ObjType:   termType,
		})
	}

	skolemizedQuads := p.rewriter.SkolemizeQuads(rawQuads, mainObjectIRI)

	return skolemizedQuads, nil
}

// evaluateNode parses concrete value types and delegates complexity to dedicated helpers.
func evaluateNode(node ld.Node, predicate string) (string, model.TermType) {
	objVal := node.GetValue()

	switch n := node.(type) {
	case ld.BlankNode:
		return objVal, model.BlankNode
	case ld.IRI:
		return objVal, model.NamedNode
	case ld.Literal:
		return formatLiteralNode(n, predicate, objVal)
	default:
		if strings.HasPrefix(objVal, "_:") {
			return objVal, model.BlankNode
		}

		if isTextMetadataPredicate(predicate) {
			return objVal, model.Literal
		}

		return objVal, model.NamedNode
	}
}

// formatLiteralNode extracts temporal formatting metrics and sanitizes string types.
func formatLiteralNode(n ld.Literal, predicate, objVal string) (string, model.TermType) {
	dt := n.Datatype

	isW3CDateTime := (strings.HasPrefix(dt, w3cXMLSchemaNS) || strings.HasPrefix(dt, shortHTTPSW3C)) && strings.Contains(dt, "dateTime")
	if strings.Contains(predicate, "published") || strings.Contains(predicate, "updated") || isW3CDateTime {
		if !strings.Contains(objVal, "^^") {
			return fmt.Sprintf(`"%s"^^<%s>`, n.Value, w3cDateTimeType), model.Literal
		}
		return objVal, model.Literal
	}

	if shouldAppendDatatype(dt) {
		objVal = fmt.Sprintf(`"%s"^^<%s>`, n.Value, dt)
	}
	return objVal, model.Literal
}

// shouldAppendDatatype filters out standard text vocabularies and shorthand stubs.
func shouldAppendDatatype(dt string) bool {
	if dt == "" {
		return false
	}
	if strings.HasPrefix(dt, shortHTTPW3C) || strings.HasPrefix(dt, shortHTTPSW3C) {
		return false
	}
	return !strings.HasPrefix(dt, w3cXMLSchemaNS)
}

// isTextMetadataPredicate verifies target text terms for loose untyped objects.
func isTextMetadataPredicate(predicate string) bool {
	return strings.Contains(predicate, "content") || strings.Contains(predicate, "summary") || strings.Contains(predicate, "name")
}
