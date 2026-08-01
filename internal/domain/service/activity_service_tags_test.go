package service

import (
	"context"
	"testing"
)

func TestActivityService_ExtractTagsAndAttachments_Standard(t *testing.T) {
	s := &ActivityService{}
	ctx := context.Background()

	payload := []byte(`{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type": "Create",
		"actor": "https://example.com/users/bob",
		"object": {
			"id": "https://example.com/users/bob/statuses/123",
			"type": "Note",
			"content": "This is a cool post with a quote and emoji!",
			"tag": [
				{
					"type": "Link",
					"href": "https://another.com/users/alice/statuses/456",
					"rel": "https://misskey-hub.net/ns#_misskey_quote",
					"mediaType": "application/ld+json; profile=\"https://www.w3.org/ns/activitystreams\""
				},
				{
					"type": "Emoji",
					"id": "https://example.com/emoji/blobcat",
					"name": ":blobcat:",
					"icon": {
						"type": "Image",
						"url": "https://example.com/emoji/blobcat.png"
					}
				}
			]
		}
	}`)

	quotes, emojis, err := s.ExtractTagsAndAttachments(ctx, payload)
	if err != nil {
		t.Fatalf("ExtractTagsAndAttachments failed: %v", err)
	}

	if len(quotes) != 1 {
		t.Errorf("Expected 1 quote, got %d", len(quotes))
	} else if quotes[0] != "https://another.com/users/alice/statuses/456" {
		t.Errorf("Unexpected quote IRI: %q", quotes[0])
	}

	if len(emojis) != 1 {
		t.Errorf("Expected 1 emoji, got %d", len(emojis))
	} else {
		emoji := emojis[0]
		if emoji.ID != "https://example.com/emoji/blobcat" {
			t.Errorf("Unexpected emoji ID: %q", emoji.ID)
		}
		if emoji.Name != ":blobcat:" {
			t.Errorf("Unexpected emoji Name: %q", emoji.Name)
		}
		if emoji.IconURL != "https://example.com/emoji/blobcat.png" {
			t.Errorf("Unexpected emoji IconURL: %q", emoji.IconURL)
		}
	}
}

func TestActivityService_ExtractTagsAndAttachments_SingleTag(t *testing.T) {
	s := &ActivityService{}
	ctx := context.Background()

	payload := []byte(`{
		"type": "Note",
		"id": "https://example.com/statuses/1",
		"tag": {
			"type": "Emoji",
			"name": ":blobfox:",
			"icon": {
				"type": "Image",
				"url": "https://example.com/emoji/blobfox.png"
			}
		}
	}`)

	quotes, emojis, err := s.ExtractTagsAndAttachments(ctx, payload)
	if err != nil {
		t.Fatalf("ExtractTagsAndAttachments failed: %v", err)
	}

	if len(quotes) != 0 {
		t.Errorf("Expected 0 quotes, got %d", len(quotes))
	}

	if len(emojis) != 1 {
		t.Errorf("Expected 1 emoji, got %d", len(emojis))
	} else if emojis[0].Name != ":blobfox:" || emojis[0].IconURL != "https://example.com/emoji/blobfox.png" {
		t.Errorf("Unexpected emoji: %+v", emojis[0])
	}
}
