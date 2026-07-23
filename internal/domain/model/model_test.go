package model_test

import (
	"testing"

	"sprezz/internal/domain/model"
)

func TestQuad_IsLiteral(t *testing.T) {
	tests := []struct {
		name     string
		termType model.TermType
		expected bool
	}{
		{
			name:     "NamedNode is not literal",
			termType: model.NamedNode,
			expected: false,
		},
		{
			name:     "BlankNode is not literal",
			termType: model.BlankNode,
			expected: false,
		},
		{
			name:     "Literal is literal",
			termType: model.Literal,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := model.Quad{
				GraphID:   1,
				Subject:   "https://example.com/actor",
				Predicate: "https://www.w3.org/ns/activitystreams#name",
				Object:    "Alice",
				ObjType:   tt.termType,
			}
			if got := q.IsLiteral(); got != tt.expected {
				t.Errorf("Quad.IsLiteral() = %v, want %v", got, tt.expected)
			}
		})
	}
}
