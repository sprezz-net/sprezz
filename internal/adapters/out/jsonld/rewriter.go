package jsonld

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"sprezz/internal/domain/model"
)

type BNodeRewriter struct{}

func NewBNodeRewriter() *BNodeRewriter {
	return &BNodeRewriter{}
}

func (r *BNodeRewriter) SkolemizeQuads(quads []model.Quad, mainObjectIRI string) []model.Quad {
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
	sort.Slice(ordered, func(i, j int) bool {
		return signatures[ordered[i]] < signatures[ordered[j]]
	})
	for index, bnodeID := range ordered {
		shortPredicate := extractShortPredicate(firstPredicate(quads, bnodeID))
		if index < 5 {
			bnodeMap[bnodeID] = mainObjectIRI + "#bnode:" + shortPredicate + ":" + stableIndex(signatures[bnodeID], index)
		} else {
			hash := sha256.Sum256([]byte(signatures[bnodeID]))
			bnodeMap[bnodeID] = mainObjectIRI + "#bnode:" + shortPredicate + ":" + hex.EncodeToString(hash[:8])
		}
	}
	return bnodeMap
}

func rewriteQuad(quad model.Quad, bnodeMap map[string]string) model.Quad {
	// Decoupled evaluation blocks to safely translate Subject and Object positions
	// independently, preventing type state corruption.
	if strings.HasPrefix(quad.Subject, "_:") {
		quad.Subject = bnodeMap[quad.Subject]
	}

	if quad.ObjType == model.BlankNode || strings.HasPrefix(quad.Object, "_:") {
		quad.Object = bnodeMap[quad.Object]
		quad.ObjType = model.NamedNode
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
	predicate := ""
	for _, quad := range quads {
		if quad.Subject == bnodeID || quad.Object == bnodeID {
			if predicate == "" || quad.Predicate < predicate {
				predicate = quad.Predicate
			}
		}
	}
	return predicate
}

func stableIndex(signature string, fallback int) string {
	hash := sha256.Sum256([]byte(signature))
	return hex.EncodeToString(hash[:2]) + "-" + strconv.Itoa(fallback)
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
