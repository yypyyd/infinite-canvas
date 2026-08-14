package service

import (
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
