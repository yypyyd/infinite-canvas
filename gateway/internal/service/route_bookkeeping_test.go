package service

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/chatgpt"
)

func TestRouteBookkeepingSkipsAcceptedTemporaryDemotion(t *testing.T) {
	class, ok, record := routeBookkeepingAfterDispatch("accepted", "temporary", chatgpt.ErrTaskAccepted)
	if record || class != "" || ok {
		t.Fatalf("accepted must not demote binding: class=%q ok=%v record=%v", class, ok, record)
	}
	class, ok, record = routeBookkeepingAfterDispatch("failed", "temporary", chatgpt.ErrTemporaryUpstream)
	if !record || class != "temporary" || ok {
		t.Fatalf("temporary failure must demote: class=%q ok=%v record=%v", class, ok, record)
	}
	class, ok, record = routeBookkeepingAfterDispatch("succeeded", "", nil)
	if !record || class != "" || !ok {
		t.Fatalf("success must clear demotion: class=%q ok=%v record=%v", class, ok, record)
	}
	class, ok, record = routeBookkeepingAfterDispatch("unknown", "temporary", errors.New("ambiguous"))
	if !record || class != "temporary" || ok {
		t.Fatalf("unknown temporary must demote: class=%q ok=%v record=%v", class, ok, record)
	}
}

func TestDispatchFailureClassAcceptedStaysTemporaryWithoutFailover(t *testing.T) {
	state, class := dispatchFailureClass(chatgpt.ErrTaskAccepted)
	if state != "accepted" || class != "temporary" {
		t.Fatalf("got state=%s class=%s", state, class)
	}
	if _, _, record := routeBookkeepingAfterDispatch(state, class, chatgpt.ErrTaskAccepted); record {
		t.Fatal("accepted temporary class must not write account_model_routes.cooldown_until")
	}
}
