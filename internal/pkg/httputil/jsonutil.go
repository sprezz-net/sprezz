package httputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// ValidateJSONDepth checks the nesting level of the JSON document using a streaming tokenizer
// without fully unmarshaling it, preventing O(M) memory exhaustion (JSON Bomb attack).
func ValidateJSONDepth(data []byte, maxDepth int) error {
	var depth int
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("malformed JSON payload: %w", err)
		}
		switch delim := t.(type) {
		case json.Delim:
			switch delim {
			case '{', '[':
				depth++
				if depth > maxDepth {
					return fmt.Errorf("exceeded maximum JSON-LD structure depth of %d", maxDepth)
				}
			case '}', ']':
				depth--
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced or malformed JSON structure")
	}
	return nil
}
