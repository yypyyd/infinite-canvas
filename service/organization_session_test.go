package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "infinite-canvas-service-test-")
	if err != nil {
		panic(err)
	}
	config.Cfg = config.Config{
		StorageDriver:  "sqlite",
		DatabaseDSN:    filepath.Join(dir, "service.db"),
		JWTSecret:      "service-test-secret",
		JWTExpireHours: 1,
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestOrganizationDataIsIsolated(t *testing.T) {
	userA, authA, organizationA := seedTenant(t, "isolation-a")
	_, _, organizationB := seedTenant(t, "isolation-b")
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Brand{ID: "brand-isolation-a", OrganizationID: organizationA.ID, Name: "A 品牌", Version: 1, CreatedBy: userA.ID, CreatedAt: "1", UpdatedAt: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Brand{ID: "brand-isolation-b", OrganizationID: organizationB.ID, Name: "B 品牌", Version: 1, CreatedBy: "user-isolation-b", CreatedAt: "1", UpdatedAt: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserProject{ID: "project-isolation-a", OrganizationID: organizationA.ID, UserID: userA.ID, Title: "A 画布", Data: `{}`, Version: 1, CreatedAt: "1", UpdatedAt: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserProject{ID: "project-isolation-b", OrganizationID: organizationB.ID, UserID: "user-isolation-b", Title: "B 画布", Data: `{}`, Version: 1, CreatedAt: "1", UpdatedAt: "1"}).Error; err != nil {
		t.Fatal(err)
	}

	brands, err := ListBrands(authA, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if brands.Total != 1 || len(brands.Items) != 1 || brands.Items[0].OrganizationID != organizationA.ID {
		t.Fatalf("expected only organization A brand, got %#v", brands)
	}
	workspace, err := UserWorkspace(authA)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace.Records) != 1 || workspace.Records[0].ObjectID != "project-isolation-a" {
		t.Fatalf("expected only organization A workspace record, got %#v", workspace.Records)
	}
	if _, _, err := ResolveOrganizationAccess(authA, organizationB.ID); err == nil {
		t.Fatal("expected cross-organization access to be rejected")
	}
	authA.OrganizationID = organizationB.ID
	if _, err := ListBrands(authA, model.Query{}); err == nil {
		t.Fatal("expected forged organization context to be rejected")
	}
}

func TestCurrentAuthUserRecoversValidOrganization(t *testing.T) {
	user, _, organizationA := seedTenant(t, "session-recovery")
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	organizationB := model.Organization{ID: "org-session-recovery-b", Name: "恢复企业", Slug: "org-session-recovery-b", Status: "active", Version: 1, CreatedBy: user.ID, CreatedAt: "2", UpdatedAt: "2"}
	membershipB := model.OrganizationMember{ID: "member-session-recovery-b", OrganizationID: organizationB.ID, UserID: user.ID, Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2"}
	if err := db.Create(&organizationB).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&membershipB).Error; err != nil {
		t.Fatal(err)
	}
	token, err := newToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Where("organization_id = ? AND user_id = ?", organizationA.ID, user.ID).Delete(&model.OrganizationMember{}).Error; err != nil {
		t.Fatal(err)
	}

	current, ok := CurrentAuthUser(token)
	if !ok {
		t.Fatal("expected session to remain valid after current membership is removed")
	}
	if current.OrganizationID != organizationB.ID {
		t.Fatalf("expected fallback organization %q, got %q", organizationB.ID, current.OrganizationID)
	}
}

func TestCurrentAuthUserRejectsBannedUser(t *testing.T) {
	user, _, _ := seedTenant(t, "session-banned")
	token, err := newToken(user)
	if err != nil {
		t.Fatal(err)
	}
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("status", model.UserStatusBan).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := CurrentAuthUser(token); ok {
		t.Fatal("expected banned user session to be rejected")
	}
}

func TestGenerationTaskChargeAndSuccessStayConsistent(t *testing.T) {
	user, _, organization := seedTenant(t, "generation-success")
	setTestUserCredits(t, user.ID, 10)
	task, err := BeginGenerationTask(GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, Model: "image-model", UpstreamModel: "upstream-image", ChannelName: "test", Path: "/images/generations", Modality: "image", Operation: "generate", ResolutionTier: "1k", Quantity: 1, Credits: 3})
	if err != nil {
		t.Fatal(err)
	}
	FinishGenerationTask(task, model.GenerationTaskStatusSuccess, "")

	assertTestUserCredits(t, user.ID, 7)
	logs, err := ListUserCreditLogs(user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Total != 1 || logs.Items[0].Type != model.CreditLogTypeAIConsume || logs.Items[0].Amount != -3 || logs.Items[0].Balance != 7 || logs.Items[0].RelatedID != task.ID {
		t.Fatalf("unexpected consumption log: %#v", logs.Items)
	}
	tasks, err := ListUserGenerationTasks(organization.ID, user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 1 || tasks.Items[0].Status != model.GenerationTaskStatusSuccess || tasks.Items[0].Credits != 3 {
		t.Fatalf("unexpected generation task: %#v", tasks.Items)
	}
}

func TestGenerationTaskInsufficientCreditsRollsBack(t *testing.T) {
	user, _, organization := seedTenant(t, "generation-insufficient")
	setTestUserCredits(t, user.ID, 2)
	if _, err := BeginGenerationTask(GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, Model: "image-model", Path: "/images/generations", Modality: "image", Operation: "generate", Quantity: 1, Credits: 3}); err == nil {
		t.Fatal("expected insufficient credits error")
	}

	assertTestUserCredits(t, user.ID, 2)
	logs, err := ListUserCreditLogs(user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Total != 0 {
		t.Fatalf("expected no credit log after rollback, got %#v", logs.Items)
	}
	tasks, err := ListUserGenerationTasks(organization.ID, user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 0 {
		t.Fatalf("expected no generation task after rollback, got %#v", tasks.Items)
	}
}

func TestGenerationTaskRequestIdempotencyPreventsDoubleCharge(t *testing.T) {
	user, _, organization := seedTenant(t, "generation-idempotency")
	setTestUserCredits(t, user.ID, 10)
	input := GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, RequestID: "request-generation-idempotency", Model: "image-model", Path: "/images/generations", Modality: "image", Operation: "generate", Quantity: 1, Credits: 3}
	if _, err := BeginGenerationTask(input); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginGenerationTask(input); err == nil {
		t.Fatal("expected duplicate generation request to be rejected")
	}

	assertTestUserCredits(t, user.ID, 7)
	logs, err := ListUserCreditLogs(user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Total != 1 || logs.Items[0].Amount != -3 {
		t.Fatalf("expected one consumption log, got %#v", logs.Items)
	}
	tasks, err := ListUserGenerationTasks(organization.ID, user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 1 || tasks.Items[0].RequestID != input.RequestID {
		t.Fatalf("expected one generation task for request %q, got %#v", input.RequestID, tasks.Items)
	}
}

func TestGenerationTaskFailureRefundsOnlyOnce(t *testing.T) {
	user, _, organization := seedTenant(t, "generation-refund")
	setTestUserCredits(t, user.ID, 10)
	task, err := BeginGenerationTask(GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, Model: "video-model", UpstreamModel: "upstream-video", ChannelName: "test", Path: "/videos", Modality: "video", Operation: "generate", ResolutionTier: "720p", Quantity: 1, Credits: 4})
	if err != nil {
		t.Fatal(err)
	}
	FinishGenerationTask(task, model.GenerationTaskStatusFailed, "上游连接中断")
	FinishGenerationTask(task, model.GenerationTaskStatusFailed, "重复失败回调")

	assertTestUserCredits(t, user.ID, 10)
	logs, err := ListUserCreditLogs(user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if logs.Total != 2 {
		t.Fatalf("expected one consumption and one refund log, got %#v", logs.Items)
	}
	counts := map[model.CreditLogType]int{}
	for _, item := range logs.Items {
		counts[item.Type]++
		if item.RelatedID != task.ID {
			t.Fatalf("expected credit log related to task %q, got %#v", task.ID, item)
		}
	}
	if counts[model.CreditLogTypeAIConsume] != 1 || counts[model.CreditLogTypeAIRefund] != 1 {
		t.Fatalf("expected exactly one consume and refund log, got %#v", counts)
	}
	tasks, err := ListUserGenerationTasks(organization.ID, user.ID, model.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Total != 1 || tasks.Items[0].Status != model.GenerationTaskStatusFailed || tasks.Items[0].ErrorMessage != "上游连接中断" {
		t.Fatalf("unexpected failed generation task: %#v", tasks.Items)
	}
}

func setTestUserCredits(t *testing.T, userID string, credits int) {
	t.Helper()
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", userID).Update("credits", credits).Error; err != nil {
		t.Fatal(err)
	}
}

func assertTestUserCredits(t *testing.T, userID string, expected int) {
	t.Helper()
	user, ok, err := repository.GetUserByID(userID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || user.Credits != expected {
		t.Fatalf("expected user credits %d, got %#v", expected, user)
	}
}

func seedTenant(t *testing.T, suffix string) (model.User, model.AuthUser, model.Organization) {
	t.Helper()
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-" + suffix, Username: "user-" + suffix, Password: "unused", OrganizationID: "org-" + suffix, Role: model.UserRoleUser, Group: "default", AffCode: "aff-" + suffix, Status: model.UserStatusActive, CreatedAt: "1", UpdatedAt: "1"}
	organization := model.Organization{ID: user.OrganizationID, Name: suffix, Slug: "slug-" + suffix, Status: "active", Version: 1, CreatedBy: user.ID, CreatedAt: "1", UpdatedAt: "1"}
	membership := model.OrganizationMember{ID: "member-" + suffix, OrganizationID: organization.ID, UserID: user.ID, Role: model.OrganizationRoleOwner, Version: 1, CreatedAt: "1", UpdatedAt: "1"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&organization).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	return user, model.PublicUser(user), organization
}
