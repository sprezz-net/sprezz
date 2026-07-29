package port

import "context"

// RemoteFetcher defines the driven port for fetching external federated resources over HTTP.
type RemoteFetcher interface {
	// FetchSigned retrieves a remote resource (e.g. ActivityPub Profile or WebFinger)
	// by signing the GET request with the provided keyID and privateKeyRSAPEM.
	// It accepts the Ed25519 key collectively for future protocol compatibility.
	FetchSigned(ctx context.Context, targetURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) ([]byte, error)
}
