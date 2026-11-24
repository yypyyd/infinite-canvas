package chatgpt

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yypyyd/infinite-canvas/internal/errtext"
)

var (
	ErrTaskAccepted          = errors.New("chatgpt image task accepted; recovery pending")
	ErrTaskSubmissionUnknown = errors.New("chatgpt image submission outcome unknown; do not resubmit")
)

// Keep HTTP status separate from upstream error text, which can contain credentials.
type providerHTTPError struct {
	status     int
	retryAfter string
	detail     string
	cause      error
}

func (e *providerHTTPError) Error() string { return e.cause.Error() }
func (e *providerHTTPError) Unwrap() error { return e.cause }

// HTTPFailure extracts a protocol reply after the caller has determined that
// the request failed. Accepted task errors deliberately hide their cause.
func HTTPFailure(err error) *errtext.Failure {
	var reply *providerHTTPError
	if errors.As(err, &reply) && reply.status >= 400 && reply.status <= 599 {
		return &errtext.Failure{Status: reply.status, Detail: reply.detail}
	}
	return nil
}

type imagePollError struct {
	readable, readErrors, lastHTTP, throttledReads int
	lastDetail                                     string
	accessDenied, rateLimited                      bool
	lastSnapshot                                   string
}

func (e *imagePollError) Error() string {
	return fmt.Sprintf("image_poll readable=%d read_errors=%d last_error_http=%d repeated_denial=%t throttled_reads=%d sustained_limit=%t last_snapshot=%s", e.readable, e.readErrors, e.lastHTTP, e.accessDenied, e.throttledReads, e.rateLimited, e.lastSnapshot)
}

// Record structural facts only. An absent asset doesn't establish render failure.
func imageSnapshotState(conv map[string]any) string {
	mapping, _ := conv["mapping"].(map[string]any)
	if len(mapping) == 0 {
		return "no_messages"
	}
	id, _ := conv["current_node"].(string)
	node, _ := mapping[id].(map[string]any)
	message, _ := node["message"].(map[string]any)
	if len(message) == 0 {
		return "no_current_message"
	}
	// Never log message text, upstream IDs or arbitrary metadata values.
	status := snapshotEnum(message["status"], "in_progress", "finished_successfully", "finished_with_error")
	author, _ := message["author"].(map[string]any)
	role := snapshotEnum(author["role"], "user", "tool", "assistant", "system")
	content, _ := message["content"].(map[string]any)
	kind := snapshotEnum(content["content_type"], "text", "code", "multimodal_text", "execution_output", "model_editable_context", "thoughts", "image_asset_pointer")
	toolTurns, asyncTurns, pointers := 0, 0, 0
	for _, raw := range mapping {
		n, _ := raw.(map[string]any)
		m, _ := n["message"].(map[string]any)
		author, _ := m["author"].(map[string]any)
		if author["role"] == "tool" {
			toolTurns++
		}
		meta, _ := m["metadata"].(map[string]any)
		if stringValue(meta["image_gen_task_id"]) != "" || meta["image_gen_async"] == true {
			asyncTurns++
		}
		if author["role"] != "assistant" && author["role"] != "tool" {
			continue
		}
		c, _ := m["content"].(map[string]any)
		parts, _ := c["parts"].([]any)
		for _, rawPart := range parts {
			part, _ := rawPart.(map[string]any)
			if pointer, _ := part["asset_pointer"].(string); pointer != "" {
				pointers++
			}
		}
	}
	return fmt.Sprintf("%s current_role=%s current_content=%s end_turn=%t current_error=%t tool_turns=%d async_turns=%d output_pointers=%d", status, role, kind, message["end_turn"] == true, message["error"] != nil, toolTurns, asyncTurns, pointers)

}

func snapshotEnum(value any, allowed ...string) string {
	for _, k := range allowed {
		if value == k {
			return k
		}
	}
	return "other"
}

func (e *imagePollError) Unwrap() error { return ErrTemporaryUpstream }

func ImageResultAccessError(err error) *errtext.Failure {
	var task *ImageTaskError
	var poll *imagePollError
	if errors.As(err, &task) && task.ConversationID != "" && errors.As(task.Cause, &poll) && (poll.accessDenied || poll.rateLimited) {
		return &errtext.Failure{Status: poll.lastHTTP, Detail: poll.lastDetail}
	}
	return nil
}

// ImageTaskError deliberately hides transport/auth errors after submission from
// account failover. A failed download does not authorize another generation.
type ImageTaskError struct {
	ConversationID string
	Cause          error
}

func (e *ImageTaskError) Error() string {
	if e.ConversationID == "" {
		return ErrTaskSubmissionUnknown.Error()
	}
	return ErrTaskAccepted.Error()
}

func (e *ImageTaskError) Is(target error) bool {
	if target == ErrContentPolicy {
		return errors.Is(e.Cause, ErrContentPolicy)
	}
	return (target == ErrTaskAccepted && e.ConversationID != "") ||
		(target == ErrTaskSubmissionUnknown && e.ConversationID == "")
}

func AcceptedFailure(conversationID string, cause error) error {
	return &ImageTaskError{ConversationID: strings.TrimSpace(conversationID), Cause: cause}
}

func AcceptedTaskID(err error) string {
	var task *ImageTaskError
	if errors.As(err, &task) {
		return task.ConversationID
	}
	return ""
}

// ResumeImageTask only reads the existing conversation; it never submits work.
func (c *Client) ResumeImageTask(ctx context.Context, accessToken, conversationID string) ([]byte, map[string]any, error) {
	session, err := c.newProxySession(accessToken)
	if err != nil {
		return nil, nil, AcceptedFailure(conversationID, err)
	}
	files, sediments, err := c.pollForImage(ctx, session, accessToken, conversationID, nil, nil, nil, pollBudget(ctx))
	if err != nil {
		return nil, nil, AcceptedFailure(conversationID, err)
	}
	urls, err := c.resolveImageURLs(ctx, session, accessToken, conversationID, files, sediments)
	if err != nil {
		return nil, nil, AcceptedFailure(conversationID, err)
	}
	if len(urls) == 0 {
		return nil, nil, AcceptedFailure(conversationID, ErrTemporaryUpstream)
	}
	return nil, map[string]any{"conversation_id": conversationID, "image_url": urls[0]}, nil
}

// References may be echoed by assistant/tool messages. Recover their IDs from
// user messages before inspecting generated assets, including after a restart.
func conversationReferenceIDs(conversation map[string]any) map[string]bool {
	refs := map[string]bool{}
	mapping, _ := conversation["mapping"].(map[string]any)
	for _, raw := range mapping {
		node, _ := raw.(map[string]any)
		message, _ := node["message"].(map[string]any)
		author, _ := message["author"].(map[string]any)
		if strings.EqualFold(stringValue(author["role"]), "user") {
			var files, sediments []string
			walkForIDs(message["content"], &files, &sediments)
			walkForIDs(message["metadata"], &files, &sediments)
			for _, id := range append(files, sediments...) {
				refs[id] = true
			}
		}
	}
	return refs
}

var failureStatusRE = regexp.MustCompile(`\b([a-z_]+):? ([1-5][0-9]{2})\b`)

// FailureSummary describes why an accepted task has not finished yet, for
// operator logs. It keeps only the error class, the request stage and the HTTP
// status, never response bodies, URLs or tokens.
func FailureSummary(err error) string {
	if err == nil {
		return ""
	}
	var task *ImageTaskError
	if errors.As(err, &task) && task.Cause != nil {
		err = task.Cause
	}
	var poll *imagePollError
	if errors.As(err, &poll) {
		return poll.Error()
	}
	class := "error"
	switch {
	case errors.Is(err, ErrAuth):
		class = "auth"
	case errors.Is(err, ErrQuotaExhausted):
		class = "rate_limited"
	case errors.Is(err, ErrTemporaryUpstream):
		class = "temporary"
	case errors.Is(err, ErrContentPolicy):
		class = "content_policy"
	case errors.Is(err, context.DeadlineExceeded):
		class = "deadline"
	case errors.Is(err, context.Canceled):
		class = "canceled"
	}
	if match := failureStatusRE.FindStringSubmatch(err.Error()); len(match) == 3 {
		return class + " " + match[1] + " " + match[2]
	}
	return class
}
