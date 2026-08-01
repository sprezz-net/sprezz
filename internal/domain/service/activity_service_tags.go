package service

import (
	"context"
	"encoding/json"
	"strings"
)

// CustomEmojiInfo represents parsed metadata for an FEP-9098 custom emoji.
type CustomEmojiInfo struct {
	ID      string
	Name    string
	IconURL string
}

// ExtractTagsAndAttachments parses the "tag" list from an ActivityPub object payload
// to identify and extract quote URLs (FEP-e232 / FEP-dd4b) and custom emojis (FEP-9098).
func (s *ActivityService) ExtractTagsAndAttachments(ctx context.Context, payload []byte) (quotes []string, customEmojis []CustomEmojiInfo, err error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, nil, err
	}

	targetMap := getTargetMap(raw)
	tagsArray := parseTagsSlice(targetMap)
	if len(tagsArray) == 0 {
		return nil, nil, nil
	}

	for _, tagItem := range tagsArray {
		s.processSingleTag(tagItem, &quotes, &customEmojis)
	}

	return quotes, customEmojis, nil
}

func (s *ActivityService) processSingleTag(tagItem interface{}, quotes *[]string, customEmojis *[]CustomEmojiInfo) {
	tagMap, ok := tagItem.(map[string]interface{})
	if !ok {
		return
	}

	tagType, _ := tagMap["type"].(string)
	if tagType == "" {
		return
	}

	if tagType == "Link" {
		if q, ok := parseLinkTag(tagMap); ok {
			*quotes = append(*quotes, q)
		}
	} else if tagType == "Emoji" || strings.HasSuffix(tagType, ":Emoji") {
		if emoji, ok := parseEmojiTag(tagMap); ok {
			*customEmojis = append(*customEmojis, emoji)
		}
	}
}

func getTargetMap(raw map[string]interface{}) map[string]interface{} {
	if obj, ok := raw["object"]; ok {
		if objMap, ok := obj.(map[string]interface{}); ok {
			return objMap
		}
	}
	return raw
}

func parseTagsSlice(targetMap map[string]interface{}) []interface{} {
	tagsVal, exists := targetMap["tag"]
	if !exists {
		return nil
	}

	if tagsArray, ok := tagsVal.([]interface{}); ok {
		return tagsArray
	}

	if singleTag, ok := tagsVal.(map[string]interface{}); ok {
		return []interface{}{singleTag}
	}

	return nil
}

func parseLinkTag(tagMap map[string]interface{}) (string, bool) {
	href, _ := tagMap["href"].(string)
	if href == "" {
		return "", false
	}

	rel, _ := tagMap["rel"].(string)
	mediaType, _ := tagMap["mediaType"].(string)

	isQuote := strings.Contains(rel, "quote") ||
		strings.Contains(rel, "_misskey_quote") ||
		strings.Contains(mediaType, "activitystreams") ||
		strings.Contains(mediaType, "ld+json")

	return href, isQuote
}

func parseEmojiTag(tagMap map[string]interface{}) (CustomEmojiInfo, bool) {
	name, _ := tagMap["name"].(string)
	id, _ := tagMap["id"].(string)
	iconVal, hasIcon := tagMap["icon"]

	if name == "" || !hasIcon {
		return CustomEmojiInfo{}, false
	}

	iconMap, ok := iconVal.(map[string]interface{})
	if !ok {
		return CustomEmojiInfo{}, false
	}

	iconURL, _ := iconMap["url"].(string)
	if iconURL == "" {
		return CustomEmojiInfo{}, false
	}

	return CustomEmojiInfo{
		ID:      id,
		Name:    name,
		IconURL: iconURL,
	}, true
}
