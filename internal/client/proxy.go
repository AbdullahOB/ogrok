package client

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"ogrok/internal/shared"
)

type LocalProxy struct {
	localPort int
	client    *http.Client
}

func NewLocalProxy(localPort int) *LocalProxy {
	return &LocalProxy{
		localPort: localPort,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *LocalProxy) ForwardRequest(req *shared.HTTPRequestMessage) (*shared.HTTPResponseMessage, error) {
	localURL := fmt.Sprintf("http://localhost:%d%s", p.localPort, req.Path)

	var body io.Reader
	if req.BodyBase64 != "" {
		bodyBytes, err := base64.StdEncoding.DecodeString(req.BodyBase64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode request body: %v", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequest(req.Method, localURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	for k, v := range req.Headers {
		if k == "Host" {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &shared.HTTPResponseMessage{
		RequestID:  req.RequestID,
		StatusCode: resp.StatusCode,
		Headers:    headers,
		BodyBase64: encodeBase64(respBody),
	}, nil
}
