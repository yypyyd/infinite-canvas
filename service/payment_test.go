package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestSignPaymentParametersSortsAndExcludesSignatureFields(t *testing.T) {
	first := signPaymentParameters(map[string]string{"b": "2", "a": "1", "sign": "ignored", "sign_type": "MD5", "empty": ""}, "merchant-secret")
	second := signPaymentParameters(map[string]string{"a": "1", "b": "2"}, "merchant-secret")
	if first != second || len(first) != 32 {
		t.Fatalf("unexpected payment signature: %q / %q", first, second)
	}
}

func TestParsePaymentAmountUsesExactCents(t *testing.T) {
	for input, want := range map[string]int64{"0.01": 1, "1": 100, "10.5": 1050, "999.99": 99999} {
		got, err := parsePaymentAmount(input)
		if err != nil || got != want {
			t.Fatalf("parsePaymentAmount(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "-1.00", "+1.00", "1.-1", "1.+1", "1.001", "1.a"} {
		if _, err := parsePaymentAmount(input); err == nil {
			t.Fatalf("parsePaymentAmount(%q) should fail", input)
		}
	}
}

func TestNormalizePaymentSettingUsesSupportedDefaultMethods(t *testing.T) {
	setting := normalizePaymentSetting(model.PaymentSetting{})
	if setting.GatewayURL != "https://www.ezfpy.cn" || setting.ProductName != "余额充值" || setting.CreditsPerYuan != 0 || !reflect.DeepEqual(setting.Methods, []model.PaymentMethod{model.PaymentMethodAlipay, model.PaymentMethodWxpay}) {
		t.Fatalf("unexpected normalized payment setting: %#v", setting)
	}
	if got := normalizePaymentSetting(model.PaymentSetting{CreditsPerYuan: -1}).CreditsPerYuan; got != 0 {
		t.Fatalf("negative creditsPerYuan = %d; want 0", got)
	}
	if got := normalizePaymentSetting(model.PaymentSetting{CreditsPerYuan: 250}).CreditsPerYuan; got != 250 {
		t.Fatalf("positive creditsPerYuan = %d; want 250", got)
	}
}

func TestNormalizePaymentSettingRequiresPaymentAndBalanceAmounts(t *testing.T) {
	setting := normalizePaymentSetting(model.PaymentSetting{Packages: []model.PaymentPackage{
		{ID: "special", Name: "优惠充值", AmountCents: 9800, BalanceCents: 10000},
		{ID: "missing-payment", Name: "缺支付金额", BalanceCents: 10000},
		{ID: "missing-balance", Name: "缺到账余额", AmountCents: 9800},
	}})
	if len(setting.Packages) != 1 || setting.Packages[0].ID != "special" {
		t.Fatalf("unexpected normalized packages: %#v", setting.Packages)
	}
}

func TestNormalizeAdminSettingsDisablesInvalidEnabledPayment(t *testing.T) {
	settings := normalizeAdminSettings(model.Settings{
		Private: model.PrivateSetting{Payment: model.PaymentSetting{
			Enabled:     true,
			MerchantID:  "1",
			MerchantKey: "merchant-secret",
			Packages: []model.PaymentPackage{
				{ID: "legacy", Name: "旧充值档位", AmountCents: 9800},
			},
		}},
	})
	if settings.Private.Payment.Enabled {
		t.Fatal("invalid enabled payment setting should be disabled in admin response")
	}
}

func TestValidatePaymentSettingStillRejectsEnabledSettingWithoutPackage(t *testing.T) {
	setting := normalizePaymentSetting(model.PaymentSetting{
		Enabled:     true,
		MerchantID:  "1",
		MerchantKey: "merchant-secret",
		Packages: []model.PaymentPackage{
			{ID: "legacy", Name: "旧充值档位", AmountCents: 9800},
		},
	})
	if err := validatePaymentSetting(setting); err == nil {
		t.Fatal("enabled payment setting without a valid package should fail validation")
	}
}

func TestQueryEasyPayOrderUsesMerchantOrderQueryAndAcceptsPaidResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/findorder" {
			t.Errorf("unexpected payment query request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		if r.Form.Get("order_no") != "pay-1" || r.Form.Get("type") != "2" {
			t.Errorf("unexpected payment query form: %#v", r.Form)
			http.Error(w, "unexpected form", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":200,"msg":"获取成功","data":{"id":6286,"type":"wxpay","trade_no":"trade-1","out_trade_no":"pay-1","money":"9.00","status":1}}`)
	}))
	defer server.Close()

	queryURL, err := paymentOrderQueryURL(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	paid, tradeNo, err := queryEasyPayOrder(server.Client(), queryURL,
		model.PaymentSetting{MerchantID: "6286"},
		model.PaymentOrder{OrderNo: "pay-1", Method: model.PaymentMethodWxpay, AmountCents: 900, BalanceCents: 1000},
	)
	if err != nil || !paid || tradeNo != "trade-1" {
		t.Fatalf("unexpected payment query result: paid=%v tradeNo=%q err=%v", paid, tradeNo, err)
	}
}

func TestValidatePaymentQueryResultRejectsMismatches(t *testing.T) {
	payment := model.PaymentSetting{MerchantID: "6286"}
	order := model.PaymentOrder{OrderNo: "pay-1", Method: model.PaymentMethodWxpay, AmountCents: 900, BalanceCents: 1000}
	valid := paymentQueryData{MerchantID: json.RawMessage(`6286`), Method: model.PaymentMethodWxpay, TradeNo: "trade-1", OrderNo: "pay-1", Money: "9.00", Status: 1}

	unpaid := valid
	unpaid.Status = 0
	if paid, tradeNo, err := validatePaymentQueryResult(payment, order, unpaid); err != nil || paid || tradeNo != "" {
		t.Fatalf("unpaid result should remain pending: paid=%v tradeNo=%q err=%v", paid, tradeNo, err)
	}

	cases := map[string]paymentQueryData{
		"merchant":       func() paymentQueryData { item := valid; item.MerchantID = json.RawMessage(`9999`); return item }(),
		"order":          func() paymentQueryData { item := valid; item.OrderNo = "pay-other"; return item }(),
		"method":         func() paymentQueryData { item := valid; item.Method = model.PaymentMethodAlipay; return item }(),
		"amount":         func() paymentQueryData { item := valid; item.Money = "8.99"; return item }(),
		"trade number":   func() paymentQueryData { item := valid; item.TradeNo = ""; return item }(),
		"unknown status": func() paymentQueryData { item := valid; item.Status = 2; return item }(),
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if paid, tradeNo, err := validatePaymentQueryResult(payment, order, result); err == nil || paid || tradeNo != "" {
				t.Fatalf("mismatch accepted: paid=%v tradeNo=%q err=%v", paid, tradeNo, err)
			}
		})
	}
}

func TestPaymentParametersRejectDuplicates(t *testing.T) {
	if _, err := paymentParameters(url.Values{"pid": []string{"6286", "other"}}); err == nil {
		t.Fatal("duplicate payment parameter should be rejected")
	}
}
