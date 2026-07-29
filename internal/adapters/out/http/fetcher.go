package http

import (
	"sprezz/internal/pkg/httputil"

	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type RemoteFetcherAdapter struct {
	client *http.Client
}

func NewRemoteFetcherAdapter(client *http.Client) *RemoteFetcherAdapter {
	return &RemoteFetcherAdapter{
		client: client,
	}
}

func (a *RemoteFetcherAdapter) FetchSigned(ctx context.Context, targetURL, keyID, privateKeyRSAPEM, privateKeyEd25519PEM string) ([]byte, error) {
	_ = privateKeyEd25519PEM // Suppress unused variable warning; currently only RSA signing is implemented

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(httputil.HeaderAccept, httputil.AcceptActivityPub)

	if privateKeyRSAPEM != "" && keyID != "" {
		cleanHost := req.URL.Host
		if host, _, err := net.SplitHostPort(req.URL.Host); err == nil {
			cleanHost = host
		}
		req.Header.Set(httputil.HeaderHost, cleanHost)
		dateStr := time.Now().UTC().Format(http.TimeFormat)
		req.Header.Set(httputil.HeaderDate, dateStr)

		signingString := fmt.Sprintf("(request-target): get %s\nhost: %s\ndate: %s",
			req.URL.RequestURI(), cleanHost, dateStr)

		signature, err := signStringRSA(signingString, privateKeyRSAPEM)
		if err == nil {
			sigHeaderVal := fmt.Sprintf("keyId=\"%s\",algorithm=\"rsa-sha256\",headers=\"(request-target) host date\",signature=\"%s\"",
				keyID, signature)
			req.Header.Set(httputil.HeaderSignature, sigHeaderVal)
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch remote profile context: status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
