package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type fakePoolTasks struct {
	status, abandoned       map[string]any
	statusErr, abandonErr   error
	statusCalls, abandonCnt int
}

func (f *fakePoolTasks) ImageTask(context.Context, string) (map[string]any, error) {
	f.statusCalls++
	return f.status, f.statusErr
}

func (f *fakePoolTasks) AbandonImageTask(context.Context, string) (map[string]any, error) {
	f.abandonCnt++
	return f.abandoned, f.abandonErr
}

func (f *fakePoolTasks) AbandonVideoJob(context.Context, string) (map[string]any, error) {
	f.abandonCnt++
	return f.abandoned, f.abandonErr
}

var errFakeNotFound = errors.New("fake: not found")

func usePoolTasks(t *testing.T, backend *fakePoolTasks) {
	t.Helper()
	originalBackend, originalArchiver, originalNotFound := currentPoolTasks, PoolImageArchiver, isPoolTaskNotFound
	t.Cleanup(func() {
		currentPoolTasks, PoolImageArchiver, isPoolTaskNotFound = originalBackend, originalArchiver, originalNotFound
	})
	currentPoolTasks = func() poolTaskBackend { return backend }
	isPoolTaskNotFound = func(err error) bool { return errors.Is(err, errFakeNotFound) }
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, now()) })
	settings := saved
	settings.Private.Storage.Driver, settings.Private.Storage.LocalPath = "local", t.TempDir()
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		t.Fatal(err)
	}
	PoolImageArchiver = func(ctx context.Context, task model.GenerationTask, _ any, data []any) ([]byte, []string, error) {
		png := []byte("\x89PNG\r\n\x1a\n0000000000000000")
		file, err := ArchiveGeneratedFile(ctx, model.AuthUser{ID: task.UserID, OrganizationID: task.OrganizationID}, task.ID, "image", 1, "image/png", int64(len(png)), bytes.NewReader(png))
		if err != nil {
			return nil, nil, err
		}
		return []byte(`{"data":[{"storage_key":"` + file.StorageKey + `"}]}`), []string{file.StorageKey}, nil
	}
}

// poolTask creates a charged (3 credits) pool image task created age ago.
func poolTask(t *testing.T, suffix string, age time.Duration) (model.GenerationTask, model.User) {
	t.Helper()
	user, _, organization := seedTenant(t, suffix)
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&user).Update("credits", 100).Error; err != nil {
		t.Fatal(err)
	}
	task, err := BeginGenerationTask(GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, RequestID: "req-" + suffix, Model: "gpt-image-2", UpstreamModel: "gpt-image-2", ChannelName: BuiltinPoolChannelName, Path: "/images/generations", Modality: "image", Operation: "generation", ResolutionTier: "1k", Quantity: 1, Credits: 3})
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-age).Format(time.RFC3339)
	if err := db.Model(&task).Update("created_at", created).Error; err != nil {
		t.Fatal(err)
	}
	task.CreatedAt = created
	return task, user
}

func reloadTask(t *testing.T, task model.GenerationTask) model.GenerationTask {
	t.Helper()
	saved, ok, err := repository.GetGenerationTaskByRequest(task.OrganizationID, task.UserID, task.RequestID)
	if err != nil || !ok {
		t.Fatalf("reload task: %v %v", ok, err)
	}
	return saved
}

func userCredits(t *testing.T, user model.User) int {
	t.Helper()
	db, _ := repository.DB()
	var saved model.User
	if err := db.First(&saved, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	return saved.Credits
}

func refundCount(t *testing.T, user model.User) int64 {
	t.Helper()
	db, _ := repository.DB()
	var count int64
	db.Model(&model.CreditLog{}).Where("user_id = ? AND type = ?", user.ID, model.CreditLogTypeAIRefund).Count(&count)
	return count
}

func TestPoolImageDeliveredAfterRestartKeepsSingleCharge(t *testing.T) {
	backend := &fakePoolTasks{status: map[string]any{"status": "completed", "created": 1, "data": []any{map[string]any{"b64_json": "eA=="}}}}
	usePoolTasks(t, backend)
	task, user := poolTask(t, "pool-late-ok", 3*time.Minute)
	// No waiter registered: the request handler is gone (e.g. 幻图 restarted).
	if !reconcilePoolImageTask(context.Background(), task) {
		t.Fatal("pool task must be handled")
	}
	saved := reloadTask(t, task)
	if saved.Status != model.GenerationTaskStatusSuccess || len(saved.StorageKeys) != 1 {
		t.Fatalf("status=%s keys=%v", saved.Status, saved.StorageKeys)
	}
	if userCredits(t, user) != 97 || refundCount(t, user) != 0 {
		t.Fatalf("charge must stand exactly once: credits=%d refunds=%d", userCredits(t, user), refundCount(t, user))
	}
}

func TestPoolImageWaitsWhileHandlerIsAlive(t *testing.T) {
	backend := &fakePoolTasks{status: map[string]any{"status": "failed"}}
	usePoolTasks(t, backend)
	task, user := poolTask(t, "pool-waiter", time.Hour)
	release := TrackPoolImageWaiter(task.RequestID)
	defer release()
	if !reconcilePoolImageTask(context.Background(), task) || backend.statusCalls != 0 {
		t.Fatal("maintenance must not touch a task its handler still waits on")
	}
	if reloadTask(t, task).Status != model.GenerationTaskStatusRunning || userCredits(t, user) != 97 {
		t.Fatal("task must stay running and charged")
	}
}

func TestPoolImageFailureRefundsOnce(t *testing.T) {
	backend := &fakePoolTasks{status: map[string]any{"status": "failed", "error": "provider request failed"}}
	usePoolTasks(t, backend)
	task, user := poolTask(t, "pool-failed", 2*time.Minute)
	reconcilePoolImageTask(context.Background(), task)
	// A second pass (or the handler finishing late) must not refund again.
	reconcilePoolImageTask(context.Background(), task)
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "late handler"); err == nil {
		t.Log("late failure write ignored")
	}
	if reloadTask(t, task).Status != model.GenerationTaskStatusFailed || userCredits(t, user) != 100 || refundCount(t, user) != 1 {
		t.Fatalf("credits=%d refunds=%d", userCredits(t, user), refundCount(t, user))
	}
}

func TestPoolImageRunningBeforeDeadlineIsLeftAlone(t *testing.T) {
	backend := &fakePoolTasks{status: map[string]any{"status": "in_progress"}}
	usePoolTasks(t, backend)
	task, user := poolTask(t, "pool-running", 35*time.Minute) // past the generic 30 min
	if !reconcilePoolImageTask(context.Background(), task) || backend.abandonCnt != 0 {
		t.Fatal("pool task within the pool deadline must keep waiting")
	}
	if reloadTask(t, task).Status != model.GenerationTaskStatusRunning || userCredits(t, user) != 97 {
		t.Fatal("no early refund")
	}
}

func TestPoolImageDeadlineAbandonsBeforeRefund(t *testing.T) {
	t.Run("late result delivered", func(t *testing.T) {
		backend := &fakePoolTasks{status: map[string]any{"status": "in_progress"}, abandoned: map[string]any{"status": "completed", "data": []any{map[string]any{"b64_json": "eA=="}}}}
		usePoolTasks(t, backend)
		task, user := poolTask(t, "pool-deadline-late", poolImageFinalDeadline+time.Minute)
		reconcilePoolImageTask(context.Background(), task)
		if backend.abandonCnt != 1 || reloadTask(t, task).Status != model.GenerationTaskStatusSuccess || userCredits(t, user) != 97 || refundCount(t, user) != 0 {
			t.Fatal("a result that finished while giving up must be delivered with the single charge")
		}
	})
	t.Run("abandoned and refunded", func(t *testing.T) {
		backend := &fakePoolTasks{status: map[string]any{"status": "in_progress"}, abandoned: map[string]any{"status": "failed"}}
		usePoolTasks(t, backend)
		task, user := poolTask(t, "pool-deadline-fail", poolImageFinalDeadline+time.Minute)
		reconcilePoolImageTask(context.Background(), task)
		if backend.abandonCnt != 1 || reloadTask(t, task).Status != model.GenerationTaskStatusFailed || userCredits(t, user) != 100 || refundCount(t, user) != 1 {
			t.Fatal("pool must be stopped first, then exactly one refund")
		}
	})
	t.Run("pool cannot be stopped", func(t *testing.T) {
		backend := &fakePoolTasks{status: map[string]any{"status": "in_progress"}, abandonErr: errors.New("db down")}
		usePoolTasks(t, backend)
		task, user := poolTask(t, "pool-deadline-err", poolImageFinalDeadline+time.Minute)
		reconcilePoolImageTask(context.Background(), task)
		if reloadTask(t, task).Status != model.GenerationTaskStatusRunning || userCredits(t, user) != 97 {
			t.Fatal("never refund while the pool may still deliver")
		}
	})
}

func TestPoolImageNeverReceived(t *testing.T) {
	backend := &fakePoolTasks{statusErr: errFakeNotFound}
	usePoolTasks(t, backend)
	young, youngUser := poolTask(t, "pool-missing-young", time.Minute)
	reconcilePoolImageTask(context.Background(), young)
	if reloadTask(t, young).Status != model.GenerationTaskStatusRunning || userCredits(t, youngUser) != 97 {
		t.Fatal("a just-submitted task gets a grace period")
	}
	old, oldUser := poolTask(t, "pool-missing-old", poolTaskSubmitGrace+time.Minute)
	reconcilePoolImageTask(context.Background(), old)
	if reloadTask(t, old).Status != model.GenerationTaskStatusFailed || userCredits(t, oldUser) != 100 || refundCount(t, oldUser) != 1 {
		t.Fatal("a request the pool never received is refunded once")
	}
}

func TestImageCountLimitOnlyForPoolOnlyModels(t *testing.T) {
	channels := []model.ModelChannel{
		{ID: BuiltinPoolChannelID, Name: BuiltinPoolChannelName, Enabled: true, Models: []model.ChannelModel{{Model: "gpt-image-2"}, {Model: "grok-imagine-image"}}},
		{ID: "third", Name: "third", BaseURL: "https://example.com/v1", Enabled: true, Models: []model.ChannelModel{{Model: "gpt-image-2"}}},
	}
	items := applyImageCountLimits([]model.ModelDefinition{
		{ID: "gpt-image-2", Modality: "image"}, {ID: "grok-imagine-image", Modality: "image", MaxImageCount: 7}, {ID: "grok-imagine-video", Modality: "video"},
	}, channels)
	if items[0].MaxImageCount != 0 || items[1].MaxImageCount != 1 || items[2].MaxImageCount != 0 {
		t.Fatalf("limits=%d,%d,%d", items[0].MaxImageCount, items[1].MaxImageCount, items[2].MaxImageCount)
	}
}

func TestPoolVideoWaitsForPoolDeadline(t *testing.T) {
	backend := &fakePoolTasks{abandonErr: errors.New("db down")}
	usePoolTasks(t, backend)
	task, user := poolTask(t, "pool-video", 35*time.Minute)
	task.Modality, task.UpstreamTaskID = "video", "vid-1"
	if !finalizePoolVideoTask(context.Background(), task) || backend.abandonCnt != 0 {
		t.Fatal("pool video inside its 45 min window must not hit the generic 30 min refund")
	}
	task.CreatedAt = time.Now().UTC().Add(-poolVideoFinalDeadline - time.Minute).Format(time.RFC3339)
	if !finalizePoolVideoTask(context.Background(), task) || backend.abandonCnt != 1 {
		t.Fatal("must stop the pool job before refunding; keep waiting if that fails")
	}
	if userCredits(t, user) != 97 {
		t.Fatal("no refund while the pool job may still deliver")
	}
}
