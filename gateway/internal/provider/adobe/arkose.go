package adobe

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

const fireflyArkoseKey = "BBCC314C-4937-4CCD-B0A3-FDF0F0F7603C"
const fireflyArkoseHost = "https://arks-client.adobe.com"

//go:embed arkose_encrypt.cjs
var arkoseEncryptScript string

//go:embed arkose_profile.json
var arkoseProfile json.RawMessage

var ErrGenerationSession = fmt.Errorf("%w: Adobe generation session unavailable", ErrTemporaryUpstream)

var arkoseEncryptionSlots = make(chan struct{}, 4)

type encryptedArkose struct {
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
	SessionID string `json:"sessionID"`
	BuildID   string `json:"buildID"`
	Version   string `json:"version"`
}

var arkoseSDKCache struct {
	sync.Mutex
	source  string
	expires time.Time
}

// generationSession obtains a fresh server-issued token on the same HTTP client
// and egress as submission. Node performs only the SDK's encryption; no browser
// or remote browser service is started and no stored ARP is trusted as fresh.
func generationSession(ctx context.Context, sess *tlsSession) (string, error) {
	source, err := loadArkoseSDK(ctx, sess)
	if err != nil {
		return "", err
	}
	select {
	case arkoseEncryptionSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	encrypted, err := encryptArkose(ctx, source, sess.fp.userAgent)
	<-arkoseEncryptionSlots
	if err != nil {
		return "", err
	}
	return requestArkoseSession(ctx, sess, encrypted)
}

func encryptArkose(ctx context.Context, source, userAgent string) (encryptedArkose, error) {
	var encrypted encryptedArkose
	input, err := json.Marshal(map[string]any{
		"source": source, "profile": arkoseProfile,
		"userAgent": userAgent, "publicKey": fireflyArkoseKey,
	})
	if err != nil {
		return encrypted, ErrGenerationSession
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	node := strings.TrimSpace(os.Getenv("ADOBE_NODE_BINARY"))
	if node == "" {
		node = "node"
	}
	cmd := exec.CommandContext(runCtx, node, "--max-old-space-size=128", "-e", arkoseEncryptScript)
	cmd.Stdin = bytes.NewReader(input)
	result, err := cmd.Output()
	if err != nil {
		return encrypted, fmt.Errorf("%w: SDK encryption failed; check Node.js and SDK version", ErrGenerationSession)
	}
	if json.Unmarshal(result, &encrypted) != nil || encrypted.Data == "" || encrypted.Timestamp <= 0 || encrypted.SessionID == "" || encrypted.BuildID == "" || encrypted.Version == "" {
		return encrypted, ErrGenerationSession
	}
	return encrypted, nil
}

func requestArkoseSession(ctx context.Context, sess *tlsSession, encrypted encryptedArkose) (string, error) {
	form := url.Values{
		"c": {encrypted.Data}, "public_key": {fireflyArkoseKey},
		"site": {"https://firefly.adobe.com"}, "userbrowser": {sess.fp.userAgent},
		"capi_version": {encrypted.Version}, "capi_mode": {"inline"},
		"style_theme": {"default"}, "rnd": {"0." + strconv.FormatInt(time.Now().UnixNano()%1000000000, 10)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fireflyArkoseHost+"/fc/gt2/public_key/"+fireflyArkoseKey, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrGenerationSession
	}
	req.Header = http.Header{
		"accept": {"*/*"}, "content-type": {"application/x-www-form-urlencoded; charset=UTF-8"},
		"origin": {"https://firefly.adobe.com"}, "referer": {"https://firefly.adobe.com/"},
		"user-agent": {sess.fp.userAgent}, "sec-ch-ua": {sess.fp.secCHUA},
		"sec-ch-ua-mobile": {"?0"}, "sec-ch-ua-platform": {sess.fp.platform},
		"ark-build-id": {encrypted.BuildID}, "x-ark-esync-value": {strconv.FormatInt(encrypted.Timestamp, 10)},
	}
	resp, err := sess.client.Do(req)
	if err != nil {
		return "", ErrGenerationSession
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", ErrGenerationSession
	}
	var reply struct {
		Token        string `json:"token"`
		ChallengeURL string `json:"challenge_url"`
		ProofOfWork  bool   `json:"pow"`
	}
	if json.Unmarshal(body, &reply) != nil || reply.ChallengeURL != "" || reply.ProofOfWork || !passiveArkoseToken(reply.Token) {
		// Never treat an unfinished interactive challenge as a usable token.
		return "", ErrGenerationSession
	}
	arp, err := json.Marshal(map[string]string{"sid": encrypted.SessionID, "ark": reply.Token})
	if err != nil {
		return "", ErrGenerationSession
	}
	return base64.StdEncoding.EncodeToString(arp), nil
}

func passiveArkoseToken(token string) bool {
	parts := strings.Split(token, "|")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return false
	}
	fields := map[string]string{}
	for _, part := range parts[1:] {
		if k, v, ok := strings.Cut(part, "="); ok {
			fields[k] = v
		}
	}
	return fields["pk"] == fireflyArkoseKey && fields["sup"] == "1"
}

func loadArkoseSDK(ctx context.Context, sess *tlsSession) (string, error) {
	arkoseSDKCache.Lock()
	defer arkoseSDKCache.Unlock()
	if arkoseSDKCache.source != "" && time.Now().Before(arkoseSDKCache.expires) {
		return arkoseSDKCache.source, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fireflyArkoseHost+"/v2/"+fireflyArkoseKey+"/api.js", nil)
	req.Header = http.Header{"user-agent": {sess.fp.userAgent}, "referer": {"https://firefly.adobe.com/"}, "accept": {"*/*"}}
	resp, err := sess.client.Do(req)
	if err != nil {
		return "", ErrGenerationSession
	}
	defer resp.Body.Close()
	source, err := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if err != nil || resp.StatusCode != http.StatusOK || len(source) > 2<<20 || !bytes.Contains(source, []byte(",Pn=n(4166),In=n(2389)")) {
		return "", fmt.Errorf("%w: unsupported Arkose SDK response", ErrGenerationSession)
	}
	arkoseSDKCache.source = string(source)
	arkoseSDKCache.expires = time.Now().Add(time.Hour)
	return arkoseSDKCache.source, nil
}

func isGPTImage2(modelID string) bool {
	return modelID == "firefly-gpt-image-2" || modelID == "firefly-gpt-image"
}

// A rejected submission can refresh once. A transport error or accepted job
// must never trigger another paid submission.
func retryGenerationSession(err error) bool { return errors.Is(err, ErrSystemUnderLoad) }
