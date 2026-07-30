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

// SafeExtractString extracts a string IRI/value recursively from a variety of JSON-LD / heterogeneous structures
func SafeExtractString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		keys := []string{"id", "@id", "@value"}
		for _, k := range keys {
			if s := SafeExtractString(v[k]); s != "" {
				return s
			}
		}
	case []interface{}:
		for _, item := range v {
			if s := SafeExtractString(item); s != "" {
				return s
			}
		}
	}
	return ""
}

// SafeExtractStringSlice recursively flattens nested arrays/maps/strings into a slice of strings
func SafeExtractStringSlice(val interface{}) []string {
	if val == nil {
		return nil
	}
	var result []string
	switch v := val.(type) {
	case string:
		result = append(result, v)
	case map[string]interface{}:
		keys := []string{"id", "@id", "@value"}
		for _, k := range keys {
			result = append(result, SafeExtractStringSlice(v[k])...)
		}
	case []interface{}:
		for _, item := range v {
			result = append(result, SafeExtractStringSlice(item)...)
		}
	}
	return result
}

// ExecuteOnHeterogeneousObjects recursively processes single maps or collections of maps
func ExecuteOnHeterogeneousObjects(val interface{}, fn func(map[string]interface{}) error) error {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case map[string]interface{}:
		return fn(v)
	case []interface{}:
		for _, item := range v {
			if err := ExecuteOnHeterogeneousObjects(item, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseStringOrID extracts a string IRI from a variety of JSON formats (string, nested map, or list)
func parseStringOrID(val interface{}) string {
	return SafeExtractString(val)
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
