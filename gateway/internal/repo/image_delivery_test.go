package repo

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"gorm.io/gorm"
)

func TestImageCapacityAndDelivery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.AutoMigrate(&model.TokenAccount{}, &model.EventLog{}, &model.DispatchAttempt{}, &model.AccountModelRoute{}); err != nil {
		t.Fatal(err)
	}
	const account = "image-account"
	const route = "image.test"
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.Create(&model.TokenAccount{ID: account, Pool: "chatgpt"}).Error)
	must(db.Create(&model.AccountModelRoute{ID: "binding", AccountID: account, ModelRouteID: route}).Error)
	for _, id := range []string{"slow", "next", "extra"} {
		must(db.Create(&model.EventLog{ID: id, Kind: "image", Provider: "chatgpt", Status: "pending", TS: time.Now().Add(-time.Minute)}).Error)
	}
	ctx := t.Context()
	repo := NewDispatchRepository(db)
	start := func(id string, max int) error {
		var binding model.AccountModelRoute
		if err := db.First(&binding, "id = ?", "binding").Error; err != nil {
			return err
		}
		_, err := repo.StartImageWithinLimit(ctx, id, route, account, max, model.ImageDispatchTurn{Previous: binding.ImageDispatchRound, Next: binding.ImageDispatchRound + 1})
		return err
	}
	settle := func(id, state, class, reason string, want bool) {
		t.Helper()
		changed, err := repo.SettleImageDelivery(ctx, id, state, class, reason)
		must(err)
		if changed != want {
			t.Fatalf("settle %s: changed=%v", id, changed)
		}
	}
	health := func(fails, success int) {
		t.Helper()
		var binding model.AccountModelRoute
		must(db.First(&binding, "id = ?", "binding").Error)
		if binding.ConsecutiveFails != fails || int(binding.SuccessTotal) != success {
			t.Fatalf("fails=%d successes=%d", binding.ConsecutiveFails, binding.SuccessTotal)
		}
	}
	must(start("slow", 1))
	must(repo.AcceptLatest(ctx, "slow", account, route, "confirmed-handle"))
	if err := start("next", 1); !errors.Is(err, ErrImageAccountOccupied) {
		t.Fatalf("recovering task lost slot: %v", err)
	}
	health(0, 0)
	// Route bookkeeping must commit with the event and dispatch, even on a retry.
	must(db.Exec("CREATE TRIGGER fail_route BEFORE UPDATE ON account_model_routes BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error)
	changed, settleErr := repo.SettleImageDelivery(ctx, "slow", "unresolved", "delivery_timeout", "deadline")
	if changed || settleErr == nil {
		t.Fatal("route failure did not roll back settlement")
	}
	if !errors.Is(start("next", 1), ErrImageAccountOccupied) {
		t.Fatal("rollback released an unfinished slot")
	}
	must(db.Exec("DROP TRIGGER fail_route").Error)
	settle("slow", "unresolved", "delivery_timeout", "deadline", true)
	settle("slow", "unresolved", "delivery_timeout", "deadline", false)
	settle("slow", "succeeded", "", "", false)
	health(1, 0)
	_, staleErr := repo.StartImageWithinLimit(ctx, "extra", route, account, 1, model.ImageDispatchTurn{Previous: 0, Next: 12})
	if !errors.Is(staleErr, ErrImageTurnClaimed) {
		t.Fatalf("outdated ranking admitted after slot release: %v", staleErr)
	}
	var closed model.DispatchAttempt
	must(db.First(&closed, "event_id = ?", "slow").Error)
	if closed.State != "unresolved" || closed.UpstreamTaskID != "confirmed-handle" || closed.FailureClass != "delivery_timeout" {
		t.Fatalf("timeout: %+v", closed)
	}
	// With capacity one, two independent arrivals cannot both start after release.
	var wg sync.WaitGroup
	admitted := make(chan string, 2)
	for _, id := range []string{"next", "extra"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := start(id, 1)
			if err == nil {
				admitted <- id
			} else if !errors.Is(err, ErrImageAccountOccupied) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(admitted)
	if len(admitted) != 1 {
		t.Fatalf("admitted=%d", len(admitted))
	}
	next := <-admitted
	must(repo.AcceptLatest(ctx, next, account, route, "second-handle"))
	settle(next, "succeeded", "", "", true)
	settle(next, "unresolved", "delivery_timeout", "late", false)
	health(0, 1)
}
