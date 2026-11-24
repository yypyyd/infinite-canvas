package chatgpt

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
)

type conversationReply struct {
	status int
	body   string
}

func TestRecoveryDistinguishesFinalTextFromPendingImage(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	const final = `{"current_node":"answer","mapping":{"history":{"message":{"author":{"role":"assistant"},"content":{"content_type":"code"}}},"answer":{"message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["Example response"]},"recipient":"all","status":"finished_successfully","end_turn":true}}}}`
	const image = `{"mapping":{"output":{"message":{"author":{"role":"tool"},"content":"file-service://result"}}}}`
	for _, tc := range []struct {
		name       string
		snapshots  []string
		foreground bool
		want       string
	}{
		{"confirmed text", []string{final}, false, "text"},
		{"submission still settling", []string{final}, true, "pending"},
		{"image arrives during confirmation", []string{final, image}, false, "result"},
		{"image queued", []string{strings.Replace(final, `"recipient":"all"`, `"metadata":{"image_gen_async":true}`, 1)}, false, "pending"},
		{"tool started", []string{strings.Replace(final, `"role":"assistant"`, `"role":"tool"`, 1)}, false, "pending"},
		{"tool requested", []string{strings.Replace(final, `"history":{"message":{`, `"history":{"message":{"recipient":"image_tool",`, 1)}, false, "pending"},
		{"turn in progress", []string{strings.Replace(final, "finished_successfully", "in_progress", 1)}, false, "pending"},
		{"empty final placeholder", []string{strings.Replace(final, "Example response", " ", 1)}, false, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader := &conversationReader{}
				for _, snapshot := range tc.snapshots {
					reader.replies = append(reader.replies, conversationReply{200, snapshot})
				}
				var refs map[string]bool
				if tc.foreground {
					refs = map[string]bool{}
				}
				start := time.Now()
				const budget = 2 * time.Minute
				files, _, err := new(Client).pollForImage(t.Context(), reader, "fixture", "test-conversation", nil, nil, refs, budget)
				switch tc.want {
				case "text":
					wrapped := AcceptedFailure("test-conversation", err)
					if !errors.Is(wrapped, ErrNoImageOutput) || errors.Is(wrapped, ErrContentPolicy) || !errors.Is(wrapped, ErrTaskAccepted) || time.Since(start) < 30*time.Second || time.Since(start) >= budget {
						t.Fatalf("err=%v elapsed=%s", err, time.Since(start))
					}
				case "pending":
					if err == nil || errors.Is(err, ErrNoImageOutput) || time.Since(start) < budget {
						t.Fatalf("err=%v elapsed=%s", err, time.Since(start))
					}
				case "result":
					if err != nil || len(files) != 1 || files[0] != tc.want {
						t.Fatalf("files=%v err=%v", files, err)
					}
				}
			})
		})
	}
}

type conversationReader struct {
	tlsclient.HttpClient
	replies     []conversationReply
	reads       int
	availableAt time.Time
	retryAfter  string
	readTimes   []time.Time
}

func (reader *conversationReader) Do(request *http.Request) (*http.Response, error) {
	if request.Method != "GET" || request.URL.Path != "/backend-api/conversation/test-conversation" {
		return nil, fmt.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
	}
	index := min(reader.reads, len(reader.replies)-1)
	if !reader.availableAt.IsZero() && time.Now().Before(reader.availableAt) {
		index = 0
	}
	reply := reader.replies[index]
	reader.readTimes = append(reader.readTimes, time.Now())
	reader.reads++
	return &http.Response{StatusCode: reply.status, Header: http.Header{"Retry-After": {reader.retryAfter}}, Body: io.NopCloser(strings.NewReader(reply.body))}, nil
}

func TestAcceptedImageReadOutcomes(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	denied := conversationReply{403, "private upstream response"}
	pending := conversationReply{200, `{"mapping":{}}`}
	echo := conversationReply{200, `{"mapping":{"u":{"message":{"author":{"role":"user"},"content":"file-service://input"}},"t":{"message":{"author":{"role":"tool"},"content":"file-service://input"}}}}`}
	image := conversationReply{200, `{"mapping":{"t":{"message":{"author":{"role":"tool"},"content":"file-service://output"}}}}`}
	for _, tc := range []struct {
		name   string
		reads  []conversationReply
		result string
	}{
		{"persistent forbidden", []conversationReply{denied}, "denied"},
		{"persistent unauthorized", []conversationReply{{401, ""}}, "denied"},
		{"readable conversation resets denials", []conversationReply{denied, denied, pending, echo, denied, denied, image}, "image"},
		{"temporary server error and throttling", []conversationReply{denied, denied, {503, ""}, denied, denied, {429, ""}, image}, "image"},
		{"still rendering", []conversationReply{pending}, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader := &conversationReader{replies: tc.reads}
				start := time.Now()
				const budget = 6 * time.Minute
				files, _, err := new(Client).pollForImage(t.Context(), reader, "fixture", "test-conversation", nil, nil, nil, budget)
				if tc.result == "image" {
					if err != nil || len(files) != 1 || files[0] != "output" {
						t.Fatalf("files=%v error=%v", files, err)
					}
					return
				}
				failure := AcceptedFailure("test-conversation", err)
				if err == nil || (ImageResultAccessError(failure) != nil) != (tc.result == "denied") {
					t.Fatalf("unexpected recovery decision: %s", FailureSummary(failure))
				}
				if tc.result == "denied" {
					if reader.reads != 3 || time.Since(start) >= budget || strings.Contains(FailureSummary(failure), denied.body) {
						t.Fatalf("reads=%d elapsed=%v diagnostic=%s", reader.reads, time.Since(start), FailureSummary(failure))
					}
				} else if time.Since(start) < budget || !strings.Contains(FailureSummary(failure), "read_errors=0") {
					t.Fatalf("readable pending conversation: %v", err)
				}
			})
		})
	}
}

func TestLateReadableImageRemainsRecoverable(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	synctest.Test(t, func(t *testing.T) {
		// One production request delivered after 36m20s. Replay that duration
		// with readable, pending responses followed by the generated asset.
		const delay = 36*time.Minute + 20*time.Second
		start := time.Now()
		reader := &conversationReader{availableAt: start.Add(delay), replies: []conversationReply{
			{200, `{"mapping":{}}`},
			{200, `{"mapping":{"t":{"message":{"author":{"role":"tool"},"content":"file-service://output"}}}}`},
		}}
		for time.Since(start) < 40*time.Minute {
			files, _, err := new(Client).pollForImage(t.Context(), reader, "fixture", "test-conversation", nil, nil, nil, 6*time.Minute)
			if err == nil {
				if len(files) != 1 || files[0] != "output" || time.Since(start) < delay {
					t.Fatalf("files=%v elapsed=%v", files, time.Since(start))
				}
				return
			}
			if ImageResultAccessError(AcceptedFailure("test-conversation", err)) != nil {
				t.Fatalf("stopped a readable task: %v", err)
			}
		}
		t.Fatal("missed late result")
	})
}

func TestDeniedImageReportsLastHTTPReason(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{403, `{"error":{"message":"Image Access Denied"}}`, "Image Access Denied"},
		{401, `<html>sign in</html>`, "HTTP 401 Unauthorized"},
		{403, `{"error":{"message":"credential fixture-token rejected"}}`, "HTTP 403 Forbidden"},
	} {
		synctest.Test(t, func(t *testing.T) {
			reader := &conversationReader{replies: []conversationReply{{tc.status, tc.body}}}
			_, _, err := new(Client).pollForImage(t.Context(), reader, "fixture-token", "test-conversation", nil, nil, nil, 6*time.Minute)
			accepted := AcceptedFailure("test-conversation", err)
			failure := ImageResultAccessError(accepted)
			if failure == nil || failure.Status != tc.status || failure.Error() != tc.want || HTTPFailure(accepted) != nil {
				t.Fatalf("result=%v", failure)
			}
		})
	}
}

// Production traces showed successful reads without an asset between 429s.
// A healthy pending read doesn't produce an image or end a throttling episode.
func TestThrottledResultDoesNotRestartRecovery(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	synctest.Test(t, func(t *testing.T) {
		pending := conversationReply{200, `{"mapping":{}}`}
		limited := conversationReply{429, `{"error":{"message":"Result read rate limit exceeded"}}`}
		reader := &conversationReader{replies: []conversationReply{limited, pending, limited, limited}}
		start := time.Now()
		_, _, err := new(Client).pollForImage(t.Context(), reader, "fixture", "test-conversation", nil, nil, nil, 6*time.Minute)
		failure := ImageResultAccessError(AcceptedFailure("test-conversation", err))
		if failure == nil || failure.Status != 429 || failure.Error() != "Result read rate limit exceeded" || reader.reads != 4 {
			t.Fatalf("reads=%d elapsed=%s failure=%v diagnostic=%s", reader.reads, time.Since(start), failure, FailureSummary(AcceptedFailure("test-conversation", err)))
		}
		t.Logf("settled after %d reads in %s", reader.reads, time.Since(start))
	})
}

func TestImageReadCooldownAcrossTasks(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		client := new(Client)
		limited := &conversationReader{replies: []conversationReply{{429, `{}`}}, retryAfter: "90"}
		_, _, err := client.pollForImage(t.Context(), limited, "account-a", "test-conversation", nil, nil, nil, 30*time.Second)
		if err == nil || limited.reads != 1 || time.Since(start) != 30*time.Second {
			t.Fatalf("first read count=%d, elapsed=%s", limited.reads, time.Since(start))
		}
		image := conversationReply{200, `{"mapping":{"t":{"message":{"author":{"role":"tool"},"content":"file-service://output"}}}}`}
		other := &conversationReader{replies: []conversationReply{image}}
		if _, _, err := client.pollForImage(t.Context(), other, "account-b", "test-conversation", nil, nil, nil, time.Minute); err != nil || !other.readTimes[0].Equal(start.Add(30*time.Second)) {
			t.Fatal("unrelated account delayed", err)
		}
		resumed := &conversationReader{replies: []conversationReply{image}}
		if _, _, err := client.pollForImage(t.Context(), resumed, "account-a", "test-conversation", nil, nil, nil, 2*time.Minute); err != nil || !resumed.readTimes[0].Equal(start.Add(90*time.Second)) {
			t.Fatal("Retry-After not respected across calls", err, resumed.readTimes)
		}
	})
}

func TestImageThrottleEvidenceSurvivesPollDeadline(t *testing.T) {
	t.Setenv("CHATGPT_DEBUG_DUMP", "")
	synctest.Test(t, func(t *testing.T) {
		client := new(Client)
		reader := &conversationReader{replies: []conversationReply{{429, `{}`}}, retryAfter: "70"}
		var failureStatus int
		for round := 0; round < 3; round++ {
			_, _, err := client.pollForImage(t.Context(), reader, "fixture", "test-conversation", nil, nil, nil, time.Minute)
			failure := ImageResultAccessError(AcceptedFailure("test-conversation", err))
			if failure != nil {
				failureStatus = failure.Status
				break
			}
		}
		if reader.reads != 3 || failureStatus != 429 {
			t.Fatalf("reads=%d terminal status=%d", reader.reads, failureStatus)
		}
		for i := 1; i < len(reader.readTimes); i++ {
			if reader.readTimes[i].Sub(reader.readTimes[i-1]) < 70*time.Second {
				t.Fatal("read before Retry-After")
			}
		}
	})
}

func TestGIFScenePreservedAfterSheetInstruction(t *testing.T) {
	const scene = "好看的女孩跳舞"
	gif := imagePrompt(scene, "1:1", "1K", "", "gif")
	if !strings.HasPrefix(gif, "请绘制一张静态的九宫格分镜图，用连续九帧表现以下场景和动作。\n"+scene+"\n") || !strings.Contains(gif, "3 列 3 行") || strings.Count(gif, scene) != 1 || strings.Contains(gif, "GIF") || !strings.HasSuffix(gif, "1:1") {
		t.Fatal("GIF format, scene or layout lost")
	}
	if plain := imagePrompt(scene, "", "", "", ""); plain != scene {
		t.Fatal("unexpected change to static image prompt")
	}
}
