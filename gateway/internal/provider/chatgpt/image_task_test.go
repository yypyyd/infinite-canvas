package chatgpt

import (
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

type conversationReader struct {
	tlsclient.HttpClient
	replies []conversationReply
	reads   int
}

func (reader *conversationReader) Do(request *http.Request) (*http.Response, error) {
	if request.Method != "GET" || request.URL.Path != "/backend-api/conversation/test-conversation" {
		return nil, fmt.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
	}
	reply := reader.replies[min(reader.reads, len(reader.replies)-1)]
	reader.reads++
	return &http.Response{StatusCode: reply.status, Body: io.NopCloser(strings.NewReader(reply.body))}, nil
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
		replies := make([]conversationReply, int(delay/(5*time.Second)))
		for i := range replies {
			replies[i] = conversationReply{200, `{"mapping":{}}`}
		}
		replies = append(replies, conversationReply{200, `{"mapping":{"t":{"message":{"author":{"role":"tool"},"content":"file-service://output"}}}}`})
		reader := &conversationReader{replies: replies}
		start := time.Now()
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
