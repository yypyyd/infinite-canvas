package service

import (
	"context"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

type fakePoolEvents struct{ events []gateway.PoolEvent }

func (f fakePoolEvents) UsageEvents(context.Context, time.Time) ([]gateway.PoolEvent, error) {
	return f.events, nil
}

func (f fakePoolEvents) EventsFor(_ context.Context, requestIDs, eventIDs []string) ([]gateway.PoolEvent, error) {
	wanted := map[string]bool{}
	for _, id := range append(requestIDs, eventIDs...) {
		wanted[id] = true
	}
	var out []gateway.PoolEvent
	for _, event := range f.events {
		if wanted[event.RequestID] || wanted[event.ID] {
			out = append(out, event)
		}
	}
	return out, nil
}

func TestPoolUsageJoinsCreditsByRequestID(t *testing.T) {
	usePoolTasks(t, &fakePoolTasks{})
	ok, _ := poolTask(t, "usage-ok", time.Minute)
	if err := FinishGenerationTask(ok, model.GenerationTaskStatusSuccess, ""); err != nil {
		t.Fatal(err)
	}
	failed, _ := poolTask(t, "usage-failed", time.Minute)
	if err := FinishGenerationTask(failed, model.GenerationTaskStatusFailed, "x"); err != nil {
		t.Fatal(err)
	}
	running, _ := poolTask(t, "usage-running", time.Minute)
	poolTask(t, "usage-missing", time.Minute) // 幻图 task the pool never recorded
	now := time.Now()
	events := []gateway.PoolEvent{
		{ID: "e1", RequestID: ok.RequestID, Status: "success", Model: "gpt-image-2", Provider: "chatgpt", AccountID: "acc-a", AccountEmail: "a@example.com", TS: now},
		// An earlier attempt for the same request on the same account must not double count credits.
		{ID: "e0", RequestID: ok.RequestID, Status: "failed", Model: "gpt-image-2", Provider: "chatgpt", AccountID: "acc-a", AccountEmail: "a@example.com", TS: now.Add(-time.Second)},
		{ID: "e2", RequestID: failed.RequestID, Status: "failed", Model: "gpt-image-2", Provider: "chatgpt", AccountID: "acc-b", TS: now},
		{ID: "e3", RequestID: running.RequestID, Status: "pending", Model: "gpt-image-2", Provider: "chatgpt", AccountID: "acc-a", AccountEmail: "a@example.com", TS: now},
		{ID: "e4", RequestID: "admin-test", Status: "success", Model: "gpt-image-2", AccountID: "acc-a", TS: now},
	}
	original := currentPoolEvents
	t.Cleanup(func() { currentPoolEvents = original })
	currentPoolEvents = func() poolEventSource { return fakePoolEvents{events: events} }

	report, err := BuildPoolUsage(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]PoolUsageRow{}
	for _, row := range report.Accounts {
		rows[row.Key] = row
	}
	a, b := rows["acc-a"], rows["acc-b"]
	if a.Requests != 4 || a.Succeeded != 2 || a.Pending != 1 || a.ChargedCredits != 3 || a.HeldCredits != 3 || a.RefundedCredits != 0 || a.Label != "a@example.com" {
		t.Fatalf("account a: %+v", a)
	}
	if b.Requests != 1 || b.RefundedCredits != 3 || b.ChargedCredits != 0 {
		t.Fatalf("account b: %+v", b)
	}
	if len(report.Models) != 1 || report.Models[0].ChargedCredits != 3 || report.Models[0].RefundedCredits != 3 {
		t.Fatalf("models: %+v", report.Models)
	}
	if report.Unlinked != 1 || report.Unattributed.Requests < 1 {
		t.Fatalf("unlinked=%d unattributed=%+v", report.Unlinked, report.Unattributed)
	}

	tasks := []model.GenerationTask{reloadTask(t, ok), reloadTask(t, failed), {ID: "other", ChannelName: "third", RequestID: ok.RequestID}}
	attachPoolAccounts(context.Background(), tasks)
	if tasks[0].PoolAccount != "chatgpt · a@example.com" || tasks[1].PoolAccount != "chatgpt · acc-b" || tasks[2].PoolAccount != "" {
		t.Fatalf("pool accounts: %q %q %q", tasks[0].PoolAccount, tasks[1].PoolAccount, tasks[2].PoolAccount)
	}
}
