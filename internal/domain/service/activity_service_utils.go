package service

import (
	"net/url"
	"strings"

	"sprezz/internal/domain/model"
)

// ThreadSafePredicateMap acts as a lock-free thread-safe read-only query lookup cache for target IRI properties.
type ThreadSafePredicateMap struct {
	m map[string][]string
}

// NewThreadSafePredicateMap converts a raw slice of quads into a thread-safe predicate map.
func NewThreadSafePredicateMap(quads []model.Quad) *ThreadSafePredicateMap {
	m := make(map[string][]string)
	for _, q := range quads {
		m[q.Predicate] = append(m[q.Predicate], q.Object)
	}
	return &ThreadSafePredicateMap{m: m}
}

// Get retrieves all objects matching a given predicate.
func (t *ThreadSafePredicateMap) Get(predicate string) []string {
	return t.m[predicate]
}

// HasKey checks if the given predicate exists.
func (t *ThreadSafePredicateMap) HasKey(predicate string) bool {
	_, ok := t.m[predicate]
	return ok
}

// Len returns the number of distinct predicates cached in the map.
func (t *ThreadSafePredicateMap) Len() int {
	return len(t.m)
}

// parseStringOrID extracts a string IRI from a variety of JSON formats (string, nested map, or list)
func parseStringOrID(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		if id, ok := v["id"].(string); ok {
			return id
		}
	case []interface{}:
		if len(v) > 0 {
			return parseStringOrID(v[0])
		}
	}
	return ""
}

// extractDomain extracts the lowercased host from an IRI
func extractDomain(iri string) string {
	u, err := url.Parse(iri)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// isAddressingPredicate isolates the specific W3C target addressing field checks.
func isAddressingPredicate(predicate string) bool {
	return predicate == model.PredicateTo ||
		predicate == model.PredicateCc ||
		predicate == model.PredicateBto ||
		predicate == model.PredicateBcc ||
		predicate == model.PredicateAudience
}
