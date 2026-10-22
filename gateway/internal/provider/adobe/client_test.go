package adobe

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
)

func TestBuildSubmitNonceMatchesBrowserPromptSlice(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":"test-user@AdobeID"}`)) + ".signature"
	// Expected hashes come from Node's SHA-256 over Firefly's JavaScript
	// expression `${userId}-${prompt.slice(0, 256)}`, including UTF-16 edges.
	for _, tc := range []struct{ name, prompt, want string }{
		{"short", "a blue cup", "da0497ded99afe48897a9da1506e0b4ece8a3d85182817fc924f4c29087bcef4"},
		{"whitespace", "  a cup\n", "6237966da2c8c1effcd91e0b84fd737037f066bce784403ec09c8f9d19341631"},
		{"empty", "", "23829a81e1b814fffb3fe72c54393b0e35b23514bed71833163ce778784b85fb"},
		{"long_ascii", strings.Repeat("a", 257), "451b7a2327c2206c82999d01aea3ff5d6bb3bf09d698f84962b71bebbcf13b95"},
		{"long_chinese", strings.Repeat("猫", 257), "972535322d31c28aa958dfab721db7f184df1dbbca38c8e177b12d28e6bba488"},
		{"emoji_boundary", strings.Repeat("a", 255) + "😀tail", "9cf36c8720a745212de00ff8da28a31716de9dafe07432d6e914c6b02c334388"},
		{"emoji_pairs", strings.Repeat("😀", 129), "5f97f66012fa2bfcda239f017460143476eb48b8a80163ee9286899ab3081512"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildSubmitNonce(token, tc.prompt); got != tc.want {
				t.Fatalf("nonce = %s, want %s", got, tc.want)
			}
		})
	}
}

type overloadedAdobeTransport struct{ tlsclient.HttpClient }

func (overloadedAdobeTransport) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 408, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error_code":"timeout_error","message":"system under load"}`))}, nil
}

func TestOverloadNeverMarksAdobeAccountDead(t *testing.T) {
	c := NewClient("", "")
	sess := &tlsSession{client: overloadedAdobeTransport{}, fp: currentAdobeFingerprint()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"image_submit", func() error {
			_, _, e := c.submitImage(ctx, sess, "token", "arp", "cup", submitURL, map[string]any{})
			return e
		}},
		{"video_submit", func() error {
			_, _, e := c.submitVideo(ctx, sess, "token", "arp", videoSubmitURL, map[string]any{})
			return e
		}},
		{"image_poll", func() error { _, _, e := c.pollImage(ctx, sess, sess, "token", submitURL, false); return e }},
		{"video_poll", func() error { _, _, e := c.pollVideo(ctx, sess, sess, "token", videoSubmitURL, false); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, ErrSystemUnderLoad) || !errors.Is(err, ErrTemporaryUpstream) || errors.Is(err, ErrDeadUpstream) || errors.Is(err, ErrAuth) {
				t.Fatalf("overload misclassified: %v", err)
			}
		})
	}
}

type trialExhaustedAdobeTransport struct{ tlsclient.HttpClient }

func (trialExhaustedAdobeTransport) Do(*http.Request) (*http.Response, error) {
	header := http.Header{}
	header.Set("x-access-error", "taste_exhausted")
	return &http.Response{StatusCode: 403, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":"private-upstream-response"}`))}, nil
}

func TestImage2DailyTrialExhaustionPreservesEntitlement(t *testing.T) {
	c := NewClient("", "")
	sess := &tlsSession{client: trialExhaustedAdobeTransport{}, fp: currentAdobeFingerprint()}
	payload := map[string]any{"modelId": "gpt-image", "modelVersion": "2"}
	_, _, err := c.submitImage(context.Background(), sess, "token", "arp", "cup", submitURL, payload)
	if !errors.Is(err, ErrTrialExhausted) || !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("daily trial limit lost its quota classification: %v", err)
	}
	if errors.Is(err, ErrEntitlement) || errors.Is(err, ErrAuth) || errors.Is(err, ErrDeadUpstream) || strings.Contains(err.Error(), "private-upstream") {
		t.Fatal("daily trial limit revoked access or exposed the response")
	}
}
