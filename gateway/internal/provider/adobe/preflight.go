package adobe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	http "github.com/bogdanfinn/fhttp"
)

// Firefly's general credit balance does not include the model-specific free
// taste allowance. An exhausted Kling taste can still return a generic 408 on
// submission, so check the actual generation entitlement before submitting.
func (c *Client) checkVideoPreflight(ctx context.Context, sess *tlsSession, token string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := "https://firefly-3p.ff.adobe.io/v2/models/generation-preflight?api=" + url.QueryEscape("/v2/3p-videos/generate-async")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header = http.Header{
		"authorization": {"Bearer " + token}, "x-api-key": {c.apiKey},
		"content-type": {"application/json"}, "accept": {"application/json"},
		"origin": {"https://firefly.adobe.com"}, "referer": {"https://firefly.adobe.com/"},
		"user-agent": {sess.fp.userAgent},
	}
	resp, err := sess.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: Adobe video quota preflight transport failed", ErrTemporaryUpstream)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrAuth
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: Adobe video quota preflight HTTP %d", ErrTemporaryUpstream, resp.StatusCode)
	}
	var result struct {
		Authorization struct {
			Result string `json:"result"`
			Reason string `json:"reasonCode"`
		} `json:"authorizationResult"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil || result.Authorization.Result == "" {
		return fmt.Errorf("%w: invalid Adobe video quota preflight response", ErrTemporaryUpstream)
	}
	if result.Authorization.Reason == "quota_exhausted" {
		return ErrQuotaExhausted
	}
	if result.Authorization.Result != "authorized" {
		return fmt.Errorf("%w: Adobe video preflight %s", ErrEntitlement, result.Authorization.Reason)
	}
	return nil
}
