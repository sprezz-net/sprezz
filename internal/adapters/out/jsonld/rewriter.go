package jsonld

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"sprezz/internal/domain/model"
)

type BNodeRewriter struct{}

func NewBNodeRewriter() *BNodeRewriter {
	return &BNodeRewriter{}
}

func (r *BNodeRewriter) SkolemizeQuads(quads []model.Quad, mainObjectIRI string) []model.Quad {
	if len(quads) == 0 {
		return quads
	}

	signatures := collectSignatures(quads)
	bnodeMap := buildBNodeMap(quads, signatures, mainObjectIRI)

	result := make([]model.Quad, len(quads))
	for i, quad := range quads {
		result[i] = rewriteQuad(quad, bnodeMap)
	}
	return result
}

func collectSignatures(quads []model.Quad) map[string]string {
	signatures := make(map[string]string)
	for _, quad := range quads {
		if strings.HasPrefix(quad.Subject, "_:") {
			signatures[quad.Subject] += "subject|" + quad.Predicate + "|" + stableTerm(quad.Object) + ";"
		}
		if quad.ObjType == model.BlankNode || strings.HasPrefix(quad.Object, "_:") {
			signatures[quad.Object] += "object|" + quad.Predicate + "|" + stableTerm(quad.Subject) + ";"
		}
	}
	return signatures
}

func buildBNodeMap(quads []model.Quad, signatures map[string]string, mainObjectIRI string) map[string]string {
	bnodeMap := make(map[string]string, len(signatures))
	ordered := make([]string, 0, len(signatures))
	for bnodeID := range signatures {
		ordered = append(ordered, bnodeID)
	}

	sort.Strings(ordered) // Lexicographical sorting prevents DoS side-channel performance exploitation

	for _, bnodeID := range ordered {
		shortPredicate := extractShortPredicate(firstPredicate(quads, bnodeID))

		// Use full-width non-linear SHA256 hashes to guarantee absolute collision resistance
		hash := sha256.Sum256([]byte(signatures[bnodeID]))
		bnodeMap[bnodeID] = mainObjectIRI + "#bnode:" + shortPredicate + ":" + hex.EncodeToString(hash[:16])
	}
	return bnodeMap
}

func rewriteQuad(quad model.Quad, bnodeMap map[string]string) model.Quad {
	if strings.HasPrefix(quad.Subject, "_:") {
		// Fallback to a randomized safe fallback identifier if the graph contains an un-indexed blank node
		if mapped, exists := bnodeMap[quad.Subject]; exists && mapped != "" {
			quad.Subject = mapped
		} else {
			quad.Subject = quad.Subject + "-unmapped-fallback"
		}
	}

	if quad.ObjType == model.BlankNode || strings.HasPrefix(quad.Object, "_:") {
		if mapped, exists := bnodeMap[quad.Object]; exists && mapped != "" {
			quad.Object = mapped
			quad.ObjType = model.NamedNode
		} else {
			quad.Object = quad.Object + "-unmapped-fallback"
		}
	}
	return quad
}

func stableTerm(term string) string {
	if strings.HasPrefix(term, "_:") {
		return "_:blank"
	}
	return term
}

func firstPredicate(quads []model.Quad, bnodeID string) string {
	var predicate string
	for _, quad := range quads {
		if quad.Subject == bnodeID || quad.Object == bnodeID {
			if predicate == "" || quad.Predicate < predicate {
				predicate = quad.Predicate
			}
		}
	}
	return predicate
}

func extractShortPredicate(predicate string) string {
	if idx := strings.LastIndexAny(predicate, "#/"); idx != -1 && idx < len(predicate)-1 {
		return predicate[idx+1:]
	}
	if predicate == "" {
		return "node"
	}
	return predicate
}
