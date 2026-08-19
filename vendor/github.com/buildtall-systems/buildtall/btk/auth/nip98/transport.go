package nip98

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

type Transport struct {
	Base http.RoundTripper
	Nsec string
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var payloadHash string
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("nip98 reading body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		h := sha256.Sum256(bodyBytes)
		payloadHash = hex.EncodeToString(h[:])
	}

	event, err := SignRequest(t.Nsec, req.URL.String(), req.Method, payloadHash)
	if err != nil {
		return nil, fmt.Errorf("nip98 sign: %w", err)
	}

	header, err := HeaderFromEvent(event)
	if err != nil {
		return nil, fmt.Errorf("nip98 header: %w", err)
	}

	r := req.Clone(req.Context())
	r.Header.Set("Authorization", header)

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(r)
}
