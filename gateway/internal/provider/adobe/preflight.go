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
func (c *Client) checkGenerationPreflight(ctx context.Context, sess *tlsSession, token, kind string, payload map[string]any) error {
	result, err := c.fetchGenerationPreflight(ctx, sess, token, kind, payload)
	if err != nil {
		return err
	}
	if result.Authorization.Reason == "quota_exhausted" {
		if result.Resource.Type == "firefly_limited_taste" {
			return ErrTrialExhausted
		}
		return ErrQuotaExhausted
	}
	if result.Authorization.Result != "authorized" {
		return fmt.Errorf("%w: Adobe %s preflight %s", ErrEntitlement, kind, result.Authorization.Reason)
	}
	return nil
}

type generationPreflight struct {
	Authorization struct {
		Result string `json:"result"`
		Reason string `json:"reasonCode"`
	} `json:"authorizationResult"`
	Resource struct {
		Type      string `json:"resourceType"`
		Reference string `json:"effectiveQuotaReferenceId"`
	} `json:"selectedResource"`
	Quotas []struct {
		Reference string  `json:"effectiveQuotaReferenceId"`
		Total     float64 `json:"provisionedQuota"`
		Used      float64 `json:"consumedQuota"`
	} `json:"effectiveQuotas"`
}

// FetchModelQuota reads each allowance independently without generating media.
func (c *Client) FetchModelQuota(ctx context.Context, account, token, kind string) (map[string]any, error) {
	sess, err := c.newSubmitTLSClient(account)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if kind == "image" {
		payload = BuildImagePayloadCandidates("firefly-gpt-image-2", "cost estimate", "1:1", "1K", "low", nil)[0]
	} else if kind == "video" {
		payload = BuildVideoPayload("kling-v4", "cost estimate", "9:16", 8, "720p", "frame", "", VideoInputs{})
	} else {
		return nil, ErrInvalidParams
	}
	result, err := c.fetchGenerationPreflight(ctx, sess, token, kind, payload)
	if err != nil {
		return nil, err
	}
	if result.Authorization.Result != "authorized" && result.Authorization.Reason != "quota_exhausted" {
		return nil, ErrEntitlement
	}
	for _, q := range result.Quotas {
		if q.Reference == result.Resource.Reference {
			return map[string]any{"total": q.Total, "used": q.Used, "remaining": max(0, q.Total-q.Used), "trial": result.Resource.Type == "firefly_limited_taste", "exhausted": result.Authorization.Reason == "quota_exhausted"}, nil
		}
	}
	return nil, fmt.Errorf("%w: Adobe %s model quota unavailable", ErrTemporaryUpstream, kind)
}

func (c *Client) fetchGenerationPreflight(ctx context.Context, sess *tlsSession, token, kind string, payload map[string]any) (*generationPreflight, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := "https://firefly-3p.ff.adobe.io/v2/models/generation-preflight?api=" + url.QueryEscape("/v2/3p-"+kind+"s/generate-async")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"authorization": {"Bearer " + token}, "x-api-key": {c.apiKey},
		"content-type": {"application/json"}, "accept": {"application/json"},
		"origin": {"https://firefly.adobe.com"}, "referer": {"https://firefly.adobe.com/"},
		"user-agent": {sess.fp.userAgent},
	}
	resp, err := sess.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: Adobe %s quota preflight transport failed", ErrTemporaryUpstream, kind)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuth
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: Adobe %s quota preflight HTTP %d", ErrTemporaryUpstream, kind, resp.StatusCode)
	}
	var result generationPreflight
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil || result.Authorization.Result == "" {
		return nil, fmt.Errorf("%w: invalid Adobe %s quota preflight response", ErrTemporaryUpstream, kind)
	}
	return &result, nil
}
