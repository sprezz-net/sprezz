package jsonld

import (
	"context"
	"fmt"
	"strings"

	"sprezz/internal/domain/model"
	"sprezz/internal/domain/ports"

	"github.com/piprate/json-gold/ld"
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

		subj := quad.Subject.GetValue()
		pred := quad.Predicate.GetValue()
		objVal := quad.Object.GetValue()

		var termType model.TermType
		switch quad.Object.(type) {
		case *ld.BlankNode:
			termType = model.BlankNode
		case *ld.Literal:
			termType = model.Literal
		default:
			if strings.HasPrefix(objVal, "_:") {
				termType = model.BlankNode
			} else {
				termType = model.NamedNode
			}
		}

		rawQuads = append(rawQuads, model.Quad{
			GraphID:   graphID,
			Subject:   subj,
			Predicate: pred,
			Object:    objVal,
			ObjType:   termType,
		})
	}

	skolemizedQuads := p.rewriter.SkolemizeQuads(rawQuads, mainObjectIRI)

	return skolemizedQuads, nil
}
