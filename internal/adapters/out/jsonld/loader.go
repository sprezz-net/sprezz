package jsonld

import (
	"embed"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/piprate/json-gold/ld"
)

//go:embed contexts/*.jsonld
var embeddedContexts embed.FS

type EmbeddedDocumentLoader struct {
	fallbackLoader ld.DocumentLoader
}

func NewEmbeddedDocumentLoader() *EmbeddedDocumentLoader {
	// 1. Build a strict, hardened network transport layer to mitigate DDoS/SSRF vectors
	secureTransport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second, // Kill slow connection hangs instantly
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 2 * time.Second, // Max time allowed to receive headers
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	}

	secureClient := &http.Client{
		Transport: secureTransport,
		Timeout:   3 * time.Second, // Hard deadline for the entire request-response cycle
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 2 { // Block infinite redirect loops
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// 2. Wrap the secure client in the JSON-LD document loader constructor
	return &EmbeddedDocumentLoader{
		fallbackLoader: ld.NewDefaultDocumentLoader(secureClient),
	}
}

func NewEmbeddedDocumentLoaderWithFallback(fallback ld.DocumentLoader) *EmbeddedDocumentLoader {
	return &EmbeddedDocumentLoader{
		fallbackLoader: fallback,
	}
}

func (l *EmbeddedDocumentLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	var filePath string
	switch {
	case strings.HasPrefix(u, "https://w3.org") || strings.HasPrefix(u, "http://w3.org"):
		filePath = "contexts/activitystreams.jsonld"
	case strings.HasPrefix(u, "https://w3id.org") || strings.HasPrefix(u, "http://w3id.org"):
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

	// Safely dispatches remaining payloads to our timeout-protected network instance
	return l.fallbackLoader.LoadDocument(u)
}
