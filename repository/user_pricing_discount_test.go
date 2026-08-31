package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestReplaceUserPricingDiscountsPreservesIdentityAndRollsBackWholeSet(t *testing.T) {
	database := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "pricing-user", Username: "pricing-user", AffCode: "pricing-user-code", Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	initial := []model.UserPricingDiscount{
		{ID: "pricing-old-generation", Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8, CreatedAt: "created-generation", UpdatedAt: "updated-generation"},
		{ID: "pricing-old-edit", Model: "image-model", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", Ratio: 0.7, CreatedAt: "created-edit", UpdatedAt: "updated-edit"},
	}
	if _, err := ReplaceUserPricingDiscounts(user.ID, initial); err != nil {
		t.Fatal(err)
	}
	replacement := []model.UserPricingDiscount{
		{ID: "pricing-incoming-generation", Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.6, CreatedAt: "incoming-created", UpdatedAt: "replacement-updated"},
		{ID: "pricing-new-2k", Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "2k", Ratio: 0.5, CreatedAt: "new-created", UpdatedAt: "new-updated"},
	}
	saved, err := ReplaceUserPricingDiscounts(user.ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved[0].ID != "pricing-old-generation" || saved[0].CreatedAt != "created-generation" || saved[0].Ratio != 0.6 || saved[1].ID != "pricing-new-2k" {
		t.Fatalf("unexpected replacement: %#v", saved)
	}

	broken := []model.UserPricingDiscount{
		{ID: "pricing-duplicate", Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "4k", Ratio: 0.4},
		{ID: "pricing-duplicate", Model: "video-model", Modality: "video", Operation: "generation", Unit: "second", ResolutionTier: "720p", Ratio: 0.3},
	}
	if _, err := ReplaceUserPricingDiscounts(user.ID, broken); err == nil {
		t.Fatal("expected duplicate primary key to abort replacement")
	}
	remaining, err := ListUserPricingDiscounts(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || remaining[0].ID != "pricing-old-generation" || remaining[1].ID != "pricing-new-2k" {
		t.Fatalf("failed replacement should preserve complete old set: %#v", remaining)
	}
}

func TestDeleteUserRemovesPricingDiscountsInTheSameTransaction(t *testing.T) {
	database := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "pricing-delete-user", Username: "pricing-delete-user", AffCode: "pricing-delete-code", Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	item := model.UserPricingDiscount{ID: "pricing-delete-item", UserID: user.ID, Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Ratio: 0.8, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := database.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteUser(user.ID); err != nil {
		t.Fatal(err)
	}
	var userCount, discountCount int64
	if err := database.Model(&model.User{}).Where("id = ?", user.ID).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.UserPricingDiscount{}).Where("user_id = ?", user.ID).Count(&discountCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount != 0 || discountCount != 0 {
		t.Fatalf("user deletion was not atomic: users=%d discounts=%d", userCount, discountCount)
	}
}
