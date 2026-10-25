package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestHoldSettleReleaseCreditsPersonal(t *testing.T) {
	db := setupUserWorkspaceTestDB(t)
	user := model.User{
		ID: "user-hold-phase3", Username: "hold-phase3", Status: model.UserStatusActive,
		Credits: 100, ReservedCredits: 0, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow,
	}
	org := model.Organization{
		ID: "org-hold-phase3", Name: "hold-phase3", Slug: "hold-phase3", Status: "active",
		CreditMode: model.OrganizationCreditModePersonal, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		var locked model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", org.ID).Error; err != nil {
			return err
		}
		source, err := HoldCredits(tx, &locked, user.ID, 10, workspaceTestNow)
		if err != nil {
			return err
		}
		if source != model.CreditSourcePersonal {
			t.Fatalf("expected personal source, got %s", source)
		}
		return SettleHeldCredits(tx, source, org.ID, user.ID, 7, 3, workspaceTestNow)
	})
	if err != nil {
		t.Fatal(err)
	}
	var saved model.User
	if err := db.First(&saved, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Credits != 93 || saved.ReservedCredits != 0 {
		t.Fatalf("after settle credits=%d reserved=%d", saved.Credits, saved.ReservedCredits)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		var locked model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", org.ID).Error; err != nil {
			return err
		}
		source, err := HoldCredits(tx, &locked, user.ID, 5, workspaceTestNow)
		if err != nil {
			return err
		}
		return ReleaseHeldCredits(tx, source, org.ID, user.ID, 5, workspaceTestNow)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&saved, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Credits != 93 || saved.ReservedCredits != 0 {
		t.Fatalf("after release credits=%d reserved=%d", saved.Credits, saved.ReservedCredits)
	}
}
