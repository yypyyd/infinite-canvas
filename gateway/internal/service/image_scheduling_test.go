package service

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
	"gorm.io/gorm"
)

func TestEligibleImageAccountsTakeTurns(t *testing.T) {
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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.AutoMigrate(&model.AccountModelRoute{}, &model.AccountQuotaBucket{}, &model.DispatchAttempt{}))
	route := model.ModelRoute{ID: "image.test.chatgpt", Provider: "chatgpt", QuotaBucketKey: "chatgpt.image", QuotaCosts: []byte(`{"mode":"metered","calculator":"fixed","unit":"generations","value":1}`)}
	ctx := withDispatchRoute(t.Context(), route, 1)
	service := &V1Service{models: repo.NewModelRepository(db), conc: NewConcurrencyService(nil)}
	now := time.Now()
	accounts := []model.TokenAccount{{ID: "recent", LastUsedAt: &now}, {ID: "idle", LastUsedAt: &now}, {ID: "unused"}, {ID: "empty"}}
	for i, p := range accounts {
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
		_, err = service.models.Dispatch().Start(ctx, "test-image", route.ID, selected)
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
