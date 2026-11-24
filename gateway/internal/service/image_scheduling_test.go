package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
	"gorm.io/gorm"
)

func imageSchedulingDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	if err := db.AutoMigrate(&model.TokenAccount{}, &model.EventLog{}, &model.AccountModelRoute{}, &model.AccountQuotaBucket{}, &model.DispatchAttempt{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEligibleImageAccountsTakeTurns(t *testing.T) {
	db := imageSchedulingDB(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	route := model.ModelRoute{ID: "image.test.chatgpt", Provider: "chatgpt", QuotaBucketKey: "chatgpt.image", QuotaCosts: []byte(`{"mode":"metered","calculator":"fixed","unit":"generations","value":1}`)}
	ctx := withDispatchRoute(t.Context(), route, 1)
	service := &V1Service{models: repo.NewModelRepository(db), conc: NewConcurrencyService(nil)}
	now := time.Now()
	accounts := []model.TokenAccount{{ID: "recent", LastUsedAt: &now}, {ID: "idle", LastUsedAt: &now}, {ID: "unused"}, {ID: "empty"}}
	for i, p := range accounts {
		must(db.Create(&model.TokenAccount{ID: p.ID, Pool: "chatgpt"}).Error)
		binding := model.AccountModelRoute{ID: p.ID, AccountID: p.ID, ModelRouteID: route.ID}
		if p.ID == "unused" {
			past := now.Add(-time.Hour)
			binding.ConsecutiveFails = 1
			binding.LastFailureAt = &past
			binding.CooldownUntil = &past
		}
		must(db.Create(&binding).Error)
		remaining := []float64{10, 20, 25, 0}[i]
		must(db.Create(&model.AccountQuotaBucket{ID: p.ID, AccountID: p.ID, BucketKey: route.QuotaBucketKey, Remaining: &remaining}).Error)
	}
	// "idle" has been used for text; it has never had an image assignment.
	for _, row := range []model.DispatchAttempt{
		{ID: "old-image", EventID: "one", ModelRouteID: route.ID, AccountID: "recent", State: "succeeded"},
		{ID: "failed-image", EventID: "two", ModelRouteID: route.ID, AccountID: "unused", State: "unresolved"},
		{ID: "used-for-text", EventID: "three", ModelRouteID: "text.test.chatgpt", AccountID: "idle", State: "succeeded"},
	} {
		must(db.Create(&row).Error)
	}
	accounts[0].Concurrency = 3
	ranked, err := service.routeAccounts(ctx, route, accounts)
	must(err)
	if ranked[0].ID != "idle" {
		t.Fatal("a used account's extra empty slots hid the unused account")
	}
	accounts[0].Concurrency = 0
	counts := map[string]int{}
	for i := 0; i < 12; i++ {
		candidates, err := service.routeAccounts(ctx, route, accounts)
		must(err)
		if len(candidates) != 3 {
			t.Fatalf("eligible=%d", len(candidates))
		}
		selected := candidates[0].ID
		if i == 0 && selected != "idle" {
			t.Fatalf("new to this image route: want idle, got %s", selected)
		}
		_, err = service.models.Dispatch().StartImageWithinLimit(ctx, fmt.Sprintf("test-%d", i), route.ID, selected, 1, candidates[0].ImageTurn)
		must(err)
		counts[selected]++
		for j := range accounts {
			if accounts[j].ID == selected {
				stamp := now.Add(time.Duration(i+1) * time.Minute)
				accounts[j].LastUsedAt = &stamp
			}
		}
	}
	if counts["recent"] != 4 || counts["idle"] != 4 || counts["unused"] != 4 {
		t.Fatalf("12 selections: %v", counts)
	}
	accounts[0].Weight = 5
	candidates, err := service.routeAccounts(ctx, route, accounts)
	must(err)
	if candidates[0].ID != "recent" {
		t.Fatal("lost administrative weight")
	}
	accounts[0].Weight = 0
	waitUntil := time.Now().Add(time.Hour)
	must(db.Model(&model.AccountModelRoute{}).Where("account_id = ?", "idle").Update("cooldown_until", waitUntil).Error)
	candidates, err = service.routeAccounts(ctx, route, accounts)
	must(err)
	for _, item := range candidates {
		if item.ID == "idle" {
			t.Fatal("selected a cooling account")
		}
	}

}

// Seven lightly used accounts must lead each round, without monopolizing it.
func TestImageDispatchRoundsCoverOneHundredAccounts(t *testing.T) {
	db := imageSchedulingDB(t)
	route := model.ModelRoute{ID: "image.test.chatgpt", Provider: "chatgpt"}
	ctx := withDispatchRoute(t.Context(), route, 0)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	makeService := func() *V1Service {
		return &V1Service{tokens: repo.NewTokenRepository(db), models: repo.NewModelRepository(db), conc: NewConcurrencyService(nil)}
	}
	scheduler := makeService()
	var accounts []model.TokenAccount
	var bindings []model.AccountModelRoute
	var history []model.DispatchAttempt
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("account-%03d", i)
		// Low-count accounts were all used more recently than the high-count group.
		usedAt := time.Unix(int64(i), 0)
		accounts = append(accounts, model.TokenAccount{ID: id, Pool: "chatgpt", Value: "fixture", Status: "active", LastUsedAt: &usedAt})
		bindings = append(bindings, model.AccountModelRoute{ID: id, AccountID: id, ModelRouteID: route.ID})
		prior := 10
		if i >= 93 {
			prior = 1
		}
		for j := 0; j < prior; j++ {
			key := fmt.Sprintf("%s-%d", id, j)
			history = append(history, model.DispatchAttempt{ID: key, EventID: key, AccountID: id, ModelRouteID: route.ID, State: "failed"})
		}
	}
	must(db.CreateInBatches(accounts, 20).Error)
	must(db.CreateInBatches(bindings, 20).Error)
	must(db.CreateInBatches(history, 40).Error)
	for round := 1; round <= 2; round++ {
		seen := map[string]bool{}
		for i := 0; i < 100; i++ {
			if i == 48 {
				scheduler = makeService()
			} // A new process must continue the same round.
			ranked, err := scheduler.routeAccounts(ctx, route, accounts)
			must(err)
			if i == 48 {
				ranked, err = scheduler.refreshPoolAccounts(ctx, "chatgpt", "image", ranked, nil)
				must(err)
				if len(ranked) != 100 {
					t.Fatalf("refresh lost candidates: %d", len(ranked))
				}
			}
			selected := ranked[0]
			id := selected.ID
			if seen[id] || selected.ImageTurn.Next != int64(round) {
				t.Fatalf("round=%d position=%d duplicate or wrong turn: %s %+v", round, i, id, selected.ImageTurn)
			}
			if i < 7 && id < "account-093" {
				t.Fatalf("round=%d high-count %s preceded a low-count account", round, id)
			}
			seen[id] = true
			event := model.EventLog{ID: fmt.Sprintf("new-%d-%d", round, i), Kind: "image", Provider: "chatgpt", Status: "pending", TS: time.Now()}
			must(db.Create(&event).Error)
			_, err = scheduler.models.Dispatch().StartImageWithinLimit(ctx, event.ID, route.ID, id, 1, selected.ImageTurn)
			must(err)
			must(repo.NewEventRepository(db).UpdateStatus(ctx, event.ID, "success", "", 1))
		}
		t.Logf("round %d: %d different accounts in 100 assignments", round, len(seen))
	}
}
