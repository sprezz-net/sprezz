package httputil

import (
	"strings"
	"testing"
)

func TestValidateJSONDepth(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		maxDepth  int
		expectErr bool
	}{
		{
			name:      "Simple flat object",
			json:      `{"key": "value"}`,
			maxDepth:  2,
			expectErr: false,
		},
		{
			name:      "Within limits",
			json:      `{"a": {"b": {"c": 1}}}`,
			maxDepth:  4,
			expectErr: false,
		},
		{
			name:      "Exceeds limits",
			json:      `{"a": {"b": {"c": 1}}}`,
			maxDepth:  2,
			expectErr: true,
		},
		{
			name:      "Array nested",
			json:      `[[[1]]]`,
			maxDepth:  2,
			expectErr: true,
		},
		{
			name:      "Malformed JSON",
			json:      `{"a": `,
			maxDepth:  5,
			expectErr: true,
		},
		{
			name:      "Highly nested bomb",
			json:      strings.Repeat("[", 101) + strings.Repeat("]", 101),
			maxDepth:  100,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJSONDepth([]byte(tc.json), tc.maxDepth)
			if (err != nil) != tc.expectErr {
				t.Errorf("expected error: %v, got error: %v", tc.expectErr, err)
			}
		})
	}
}
