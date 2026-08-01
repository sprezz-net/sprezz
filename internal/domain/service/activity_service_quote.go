package service

import (
	"context"
	"encoding/json"
	"fmt"

	"sprezz/internal/domain/model"
)

func (s *ActivityService) validateQuoteRequestVerb(actorIRI string, object, instrument interface{}) error {
	quotedIRI := parseStringOrID(object)
	if quotedIRI == "" {
		return fmt.Errorf("missing object for QuoteRequest")
	}

	instMap, ok := instrument.(map[string]interface{})
	if !ok {
		return nil
	}

	instAttributedTo := parseStringOrID(instMap["attributedTo"])
	if instAttributedTo == "" {
		instAttributedTo = parseStringOrID(instMap["actor"])
	}

	if instAttributedTo != "" && instAttributedTo != actorIRI {
		return fmt.Errorf("security violation: QuoteRequest actor %s does not match instrument author %s", actorIRI, instAttributedTo)
	}

	quoteVal, ok := instMap["quote"]
	if ok {
		quoteIRI := SafeExtractString(quoteVal)
		if quoteIRI != "" && quoteIRI != quotedIRI {
			return fmt.Errorf("security violation: QuoteRequest object %s does not match instrument quote %s", quotedIRI, quoteIRI)
		}
	}

	return nil
}

func (s *ActivityService) extractQuoteIRI(objMap map[string]interface{}) (string, bool) {
	for _, key := range []string{"quote", "_misskey_quote", "quoteUrl", "quoteUri"} {
		if val, exists := objMap[key]; exists {
			qIRI := SafeExtractString(val)
			if qIRI != "" {
				return qIRI, true
			}
		}
	}
	return "", false
}

func (s *ActivityService) validateQuotePostConsent(ctx context.Context, actorIRI string, object interface{}) error {
	objMap, ok := object.(map[string]interface{})
	if !ok {
		return nil
	}

	quoteIRI, hasQuote := s.extractQuoteIRI(objMap)
	if !hasQuote {
		return nil
	}

	quotePostID := SafeExtractString(objMap["id"])

	quotedAuthor, err := s.getQuotedPostAuthor(ctx, quoteIRI)
	if err != nil {
		return fmt.Errorf("privacy guard rejection: remote quoted object %s cannot be verified: %w", quoteIRI, err)
	}

	if actorIRI == quotedAuthor {
		return nil
	}

	quoteAuthVal, exists := objMap["quoteAuthorization"]
	if !exists {
		return fmt.Errorf("privacy guard: third-party quote post %s lacks a valid FEP-044f quote authorization stamp", quotePostID)
	}

	quoteAuthIRI := SafeExtractString(quoteAuthVal)
	if quoteAuthIRI == "" {
		return fmt.Errorf("privacy guard: third-party quote post %s lacks a valid FEP-044f quote authorization stamp", quotePostID)
	}

	return s.verifyQuoteAuthorization(ctx, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI)
}

func (s *ActivityService) verifyQuoteAuthorization(ctx context.Context, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI string) error {
	found, err := s.verifyLocalQuoteAuthorization(ctx, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI)
	if found {
		return err
	}
	return s.verifyRemoteQuoteAuthorization(ctx, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI)
}

func (s *ActivityService) verifyLocalQuoteAuthorization(ctx context.Context, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI string) (bool, error) {
	stampQuads, err := s.storage.StreamQuadsBySubject(ctx, quoteAuthIRI)
	if err != nil || len(stampQuads) == 0 {
		return false, nil
	}

	stampMap := NewThreadSafePredicateMap(stampQuads)
	stampType := ""
	for _, obj := range stampMap.m[model.RDFType] {
		stampType = obj
	}
	if stampType != model.TypeQuoteAuthorization && stampType != "QuoteAuthorization" {
		return true, fmt.Errorf("privacy guard: quote authorization stamp type %s is invalid", stampType)
	}

	var stampInteracting, stampTarget, stampAttributedTo string
	for pred, objects := range stampMap.m {
		if len(objects) == 0 {
			continue
		}
		cleanVal := objects[0]
		switch pred {
		case model.PredicateInteractingObject:
			stampInteracting = cleanVal
		case model.PredicateInteractionTarget:
			stampTarget = cleanVal
		case model.PredicateAttributedTo:
			stampAttributedTo = cleanVal
		}
	}

	if stampInteracting != quotePostID {
		return true, fmt.Errorf("privacy guard: stamp interactingObject %s does not match quote post %s", stampInteracting, quotePostID)
	}
	if stampTarget != quoteIRI {
		return true, fmt.Errorf("privacy guard: stamp interactionTarget %s does not match quoted post %s", stampTarget, quoteIRI)
	}
	if stampAttributedTo != quotedAuthor {
		return true, fmt.Errorf("privacy guard: stamp attributedTo %s does not match quoted author %s", stampAttributedTo, quotedAuthor)
	}
	return true, nil
}

func (s *ActivityService) verifyRemoteQuoteAuthorization(ctx context.Context, quotePostID, quoteIRI, quotedAuthor, quoteAuthIRI string) error {
	stampBytes, err := s.fetcher.FetchSigned(ctx, quoteAuthIRI, "", "", "")
	if err != nil {
		return fmt.Errorf("privacy guard rejection: remote quote authorization stamp %s cannot be fetched: %w", quoteAuthIRI, err)
	}

	var stamp struct {
		Type              string      `json:"type"`
		AttributedTo      interface{} `json:"attributedTo"`
		InteractingObject interface{} `json:"interactingObject"`
		InteractionTarget interface{} `json:"interactionTarget"`
	}
	if err := json.Unmarshal(stampBytes, &stamp); err != nil {
		return fmt.Errorf("privacy guard rejection: remote quote authorization stamp %s is malformed", quoteAuthIRI)
	}

	extractedType := stamp.Type
	extractedAttributedTo := parseStringOrID(stamp.AttributedTo)
	extractedInteracting := parseStringOrID(stamp.InteractingObject)
	extractedTarget := parseStringOrID(stamp.InteractionTarget)

	if extractedType != "QuoteAuthorization" && extractedType != model.TypeQuoteAuthorization {
		return fmt.Errorf("privacy guard: stamp type %s is invalid", extractedType)
	}
	if extractedInteracting != quotePostID {
		return fmt.Errorf("privacy guard: stamp interactingObject %s does not match quote post %s", extractedInteracting, quotePostID)
	}
	if extractedTarget != quoteIRI {
		return fmt.Errorf("privacy guard: stamp interactionTarget %s does not match quoted post %s", extractedTarget, quoteIRI)
	}
	if extractedAttributedTo != quotedAuthor {
		return fmt.Errorf("privacy guard: stamp attributedTo %s does not match quoted author %s", extractedAttributedTo, quotedAuthor)
	}

	return nil
}

func (s *ActivityService) getQuotedPostAuthor(ctx context.Context, quotedIRI string) (string, error) {
	quotedQuads, err := s.storage.StreamQuadsBySubject(ctx, quotedIRI)
	if err == nil && len(quotedQuads) > 0 {
		quotedMap := NewThreadSafePredicateMap(quotedQuads)
		author := s.getOriginalActor(quotedMap)
		if author != "" {
			return author, nil
		}
	}

	fetchedBody, err := s.fetcher.FetchSigned(ctx, quotedIRI, "", "", "")
	if err != nil {
		return "", err
	}

	var post struct {
		AttributedTo interface{} `json:"attributedTo"`
	}
	if err := json.Unmarshal(fetchedBody, &post); err != nil {
		return "", err
	}

	author := parseStringOrID(post.AttributedTo)
	if author == "" {
		return "", fmt.Errorf("missing attributedTo for quoted post %s", quotedIRI)
	}
	return author, nil
}
