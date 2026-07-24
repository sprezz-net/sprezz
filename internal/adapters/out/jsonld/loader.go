package jsonld

import (
	"embed"
	"strings"

	"github.com/piprate/json-gold/ld"
)

//go:embed contexts/*.jsonld
var embeddedContexts embed.FS

type EmbeddedDocumentLoader struct {
	fallbackLoader ld.DocumentLoader
}

func NewEmbeddedDocumentLoader() *EmbeddedDocumentLoader {
	return &EmbeddedDocumentLoader{
		// Default loader handles standard remote HTTP lookups when local contexts aren't matched
		fallbackLoader: ld.NewDefaultDocumentLoader(nil),
	}
}

func (l *EmbeddedDocumentLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	var filePath string
	switch {
	case strings.HasPrefix(u, "https://www.w3.org/ns/activitystreams") || strings.HasPrefix(u, "http://www.w3.org/ns/activitystreams"):
		filePath = "contexts/activitystreams.jsonld"
	case strings.HasPrefix(u, "https://w3id.org/security/v1") || strings.HasPrefix(u, "http://w3id.org/security/v1"):
		filePath = "contexts/security_v1.jsonld"
	}

	if filePath != "" {
		content, err := embeddedContexts.ReadFile(filePath)
		if err == nil {
			doc, parseErr := ld.DocumentFromReader(strings.NewReader(string(content)))
			if parseErr == nil {
				return &ld.RemoteDocument{
					DocumentURL: u,
					Document:    doc,
				}, nil
			}
		}
	}

	// Delegate unknown or platform-specific extensions (like Mastodon namespaces) to the fallback network loader.
	return l.fallbackLoader.LoadDocument(u)
}

// NewEmbeddedDocumentLoaderWithFallback allows injecting a mock or configured loader during testing
func NewEmbeddedDocumentLoaderWithFallback(fallback ld.DocumentLoader) *EmbeddedDocumentLoader {
	return &EmbeddedDocumentLoader{
		fallbackLoader: fallback,
	}
}
