package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestAccountCreditHTTPCheckInAndRedeemAreAtomic(t *testing.T) {
	tenant := seedRouterTestTenant(t, "account-credits")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.User{}).Where("id = ?", tenant.User.ID).Update("credits", 10).Error; err != nil {
		t.Fatal(err)
	}
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.CheckIn = model.CheckInSetting{Enabled: true, Reward: true, RewardCredits: 3}
	if _, err := repository.SaveSettings(settings, "router-account-credits-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "router-account-credits-cleanup") })

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	statusResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/check-in", nil, headers)
	var status model.CheckInStatus
	if statusResponse.Code != 0 || json.Unmarshal(statusResponse.Data, &status) != nil || !status.Enabled || !status.Reward || status.RewardCredits != 3 || status.CheckedInToday {
		t.Fatalf("unexpected check-in status: %#v, status=%#v", statusResponse, status)
	}
	checkInResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/check-in", nil, headers)
	var checkIn model.CheckInResult
	if checkInResponse.Code != 0 || json.Unmarshal(checkInResponse.Data, &checkIn) != nil || checkIn.RewardCredits != 3 || checkIn.User.Credits != 13 || checkIn.User.OrganizationID != tenant.Organization.ID {
		t.Fatalf("unexpected check-in response: %#v, result=%#v", checkInResponse, checkIn)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/check-in", nil, headers); response.Code != 1 {
		t.Fatalf("repeated check-in response: %#v", response)
	}

	redemption := model.RedemptionCode{
		ID: "redeem-router-account-credits", Code: "ROUTER-REDEEM-0001", Credits: 5,
		Status: model.RedemptionCodeStatusActive, Remark: "router test", CreatedAt: "1", UpdatedAt: "1",
	}
	if err := database.Create(&redemption).Error; err != nil {
		t.Fatal(err)
	}
	redeemResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/redeem-codes/redeem", map[string]string{"code": "router - redeem - 0001"}, headers)
	var redeemedUser model.AuthUser
	if redeemResponse.Code != 0 || json.Unmarshal(redeemResponse.Data, &redeemedUser) != nil || redeemedUser.Credits != 18 || redeemedUser.OrganizationID != tenant.Organization.ID {
		t.Fatalf("unexpected redeem response: %#v, user=%#v", redeemResponse, redeemedUser)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/redeem-codes/redeem", map[string]string{"code": redemption.Code}, headers); response.Code != 1 {
		t.Fatalf("repeated redeem response: %#v", response)
	}

	var savedUser model.User
	var savedCode model.RedemptionCode
	if err := database.First(&savedUser, "id = ?", tenant.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&savedCode, "id = ?", redemption.ID).Error; err != nil {
		t.Fatal(err)
	}
	if savedUser.Credits != 18 || savedCode.Status != model.RedemptionCodeStatusUsed || savedCode.UsedBy != tenant.User.ID || savedCode.UsedAt == "" {
		t.Fatalf("unexpected persisted credit state: user=%#v, code=%#v", savedUser, savedCode)
	}
	var checkIns int64
	if err := database.Model(&model.CheckIn{}).Where("user_id = ?", tenant.User.ID).Count(&checkIns).Error; err != nil || checkIns != 1 {
		t.Fatalf("check-in count = %d, err=%v", checkIns, err)
	}
	var logs []model.CreditLog
	if err := database.Where("user_id = ?", tenant.User.ID).Order("created_at asc").Find(&logs).Error; err != nil || len(logs) != 2 {
		t.Fatalf("credit logs: %#v, err=%v", logs, err)
	}
	amounts := map[model.CreditLogType]int{}
	balances := map[model.CreditLogType]int{}
	for _, log := range logs {
		amounts[log.Type], balances[log.Type] = log.Amount, log.Balance
	}
	if amounts[model.CreditLogTypeCheckIn] != 3 || balances[model.CreditLogTypeCheckIn] != 13 || amounts[model.CreditLogTypeRedeem] != 5 || balances[model.CreditLogTypeRedeem] != 18 {
		t.Fatalf("unexpected credit accounting: logs=%#v", logs)
	}
}
