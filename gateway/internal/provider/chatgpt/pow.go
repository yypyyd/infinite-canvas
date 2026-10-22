package chatgpt

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/sha3"
)

var (
	cores             = []int{8, 16, 24, 32}
	documentKeys      = []string{"__reactContainer$fzelfjyxej8", "_reactListening5dehydibo78", "location"}
	screenResolutions = [][2]int{{1920, 1080}, {1440, 900}, {2560, 1440}, {3840, 2160}}
	navKeys           = []string{
		"registerProtocolHandler−function registerProtocolHandler() { [native code] }",
		"storage−[object StorageManager]",
		"locks−[object LockManager]",
		"appCodeName−Mozilla",
		"permissions−[object Permissions]",
		"share−function share() { [native code] }",
		"webdriver−false",
		"vendor−Google Inc.",
		"mediaDevices−[object MediaDevices]",
		"cookieEnabled−true",
		"onLine−true",
		"mimeTypes−[object MimeTypeArray]",
		"credentials−[object CredentialsContainer]",
		"serviceWorker−[object ServiceWorkerContainer]",
		"keyboard−[object Keyboard]",
		"gpu−[object GPU]",
		"doNotTrack",
		"language−zh-CN",
		"geolocation−[object Geolocation]",
		"hardwareConcurrency−32",
	}
	winKeys = []string{
		"0", "window", "self", "document", "name", "location", "history",
		"navigation", "innerWidth", "innerHeight", "screen", "chrome",
		"navigator", "performance", "crypto", "indexedDB", "sessionStorage",
		"localStorage", "fetch", "matchMedia", "postMessage", "setTimeout",
		"caches", "__NEXT_DATA__",
	}
)

func buildLegacyRequirementsToken(userAgent string, scriptSources []string, dataBuild string) string {
	cfg := buildPOWConfig(userAgent, scriptSources, dataBuild)
	body, _ := json.Marshal(cfg)
	return "gAAAAAC" + base64.StdEncoding.EncodeToString(body)
}

func buildProofToken(seed, difficulty, userAgent string, scriptSources []string, dataBuild string) (string, error) {
	cfg := buildPOWConfig(userAgent, scriptSources, dataBuild)
	answer, solved := powGenerate(seed, difficulty, cfg, 500000)
	if !solved {
		return "", errors.New("failed to solve proof token")
	}
	return "gAAAAAB" + answer, nil
}

func buildPOWConfig(userAgent string, scriptSources []string, dataBuild string) []any {
	// scriptSources/dataBuild are no longer part of the sentinel config array
	// (the current chatgpt.com client dropped them); kept in the signature for
	// call-site compatibility.
	_ = scriptSources
	_ = dataBuild
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	screen := screenResolutions[rng.Intn(len(screenResolutions))]
	loc := time.FixedZone("GMT+0800", 8*3600)
	nowLocal := time.Now().In(loc).Format("Mon Jan 02 2006 15:04:05") + " GMT+0800 (中国标准时间)"
	perf := float64(time.Now().UnixNano()%1_000_000_000) / 1_000_000
	return []any{
		screen[0] + screen[1],  // [0]
		nowLocal,               // [1] local time, JS Date.toString() shape
		4395630592,             // [2]
		1,                      // [3] overwritten by powGenerate counter
		userAgent,              // [4]
		nil,                    // [5] (was script source; now null)
		defaultClientVersion,   // [6] oai-client-version, must match header
		"zh-CN",                // [7] matches oai-language
		"zh-CN,en,en-GB,en-US", // [8]
		rng.Float64(),          // [9] overwritten by powGenerate counter
		navKeys[rng.Intn(len(navKeys))],
		documentKeys[rng.Intn(len(documentKeys))],
		winKeys[rng.Intn(len(winKeys))],
		perf,                         // [13]
		newUUID(),                    // [14]
		"",                           // [15]
		cores[rng.Intn(len(cores))],  // [16]
		float64(timeMillis()) - perf, // [17]
		0, 0, 0, 0, 0, 0,
		0,
	}
}

func powGenerate(seed, difficulty string, cfg []any, limit int) (string, bool) {
	target, err := hex.DecodeString(strings.TrimSpace(difficulty))
	if err != nil {
		return "", false
	}
	diffLen := len(strings.TrimSpace(difficulty)) / 2
	seedBytes := []byte(seed)
	head1, _ := json.Marshal(cfg[:3])
	head2, _ := json.Marshal(cfg[4:9])
	head3, _ := json.Marshal(cfg[10:])
	static1 := []byte(string(head1[:len(head1)-1]) + ",")
	static2 := []byte("," + string(head2[1:len(head2)-1]) + ",")
	static3 := []byte("," + string(head3[1:]))

	for i := 0; i < limit; i++ {
		finalJSON := append([]byte{}, static1...)
		finalJSON = append(finalJSON, []byte(strconvItoa(i))...)
		finalJSON = append(finalJSON, static2...)
		finalJSON = append(finalJSON, []byte(strconvItoa(i>>1))...)
		finalJSON = append(finalJSON, static3...)
		encoded := base64.StdEncoding.EncodeToString(finalJSON)
		sum := sha3.Sum512(append(seedBytes, []byte(encoded)...))
		if bytesCompare(sum[:diffLen], target) <= 0 {
			return encoded, true
		}
	}
	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func bytesCompare(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func strconvItoa(v int) string {
	return strconv.Itoa(v)
}
