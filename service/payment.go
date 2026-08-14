package service

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func GetPaymentConfig() (model.PaymentConfig, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.PaymentConfig{}, err
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	enabled := validatePaymentSetting(payment) == nil && payment.Enabled
	return model.PaymentConfig{Enabled: enabled, Methods: payment.Methods, Packages: payment.Packages, CreditsPerYuan: payment.CreditsPerYuan}, nil
}

func CreatePaymentOrder(user model.AuthUser, packageID string, method model.PaymentMethod) (model.PaymentSubmission, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.PaymentSubmission{}, err
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	if err := validatePaymentSetting(payment); err != nil || !payment.Enabled {
		return model.PaymentSubmission{}, safeMessageError{message: "在线充值暂未开放"}
	}
	if !paymentMethodEnabled(payment.Methods, method) {
		return model.PaymentSubmission{}, safeMessageError{message: "不支持所选支付方式"}
	}
	selected, ok := paymentPackageByID(payment.Packages, strings.TrimSpace(packageID))
	if !ok {
		return model.PaymentSubmission{}, safeMessageError{message: "充值档位不存在"}
	}
	baseURL, err := paymentPublicBaseURL()
	if err != nil {
		return model.PaymentSubmission{}, err
	}
	timestamp := now()
	order := model.PaymentOrder{
		ID: newID("payment"), OrderNo: newID("pay"), UserID: user.ID, OrganizationID: user.OrganizationID,
		PackageID: selected.ID, PackageName: selected.Name, Method: method, AmountCents: selected.AmountCents, BalanceCents: selected.BalanceCents,
		Status: model.PaymentOrderStatusPending, CreatedAt: timestamp, UpdatedAt: timestamp,
	}
	order, err = repository.CreatePaymentOrder(order)
	if err != nil {
		return model.PaymentSubmission{}, err
	}
	params := map[string]string{
		"pid": payment.MerchantID, "type": string(method), "out_trade_no": order.OrderNo,
		"notify_url": baseURL + "/api/payments/notify", "return_url": baseURL + "/api/payments/return",
		"name": payment.ProductName + " - " + selected.Name, "money": formatPaymentAmount(selected.AmountCents),
		"sitename": payment.SiteName,
	}
	params["sign"] = signPaymentParameters(params, payment.MerchantKey)
	params["sign_type"] = "MD5"
	return model.PaymentSubmission{Order: order, SubmitURL: paymentSubmitURL(payment.GatewayURL), Params: params}, nil
}

func ListPaymentOrders(userID string, q model.Query) (model.PaymentOrderList, error) {
	items, total, err := repository.ListUserPaymentOrders(userID, q)
	if err != nil {
		return model.PaymentOrderList{}, err
	}
	return model.PaymentOrderList{Items: items, Total: int(total)}, nil
}

func GetPaymentOrder(userID string, orderNo string) (model.PaymentOrder, error) {
	order, err := repository.GetUserPaymentOrder(userID, strings.TrimSpace(orderNo))
	if errors.Is(err, repository.ErrPaymentOrderNotFound) {
		return model.PaymentOrder{}, safeMessageError{message: "支付订单不存在"}
	}
	return order, err
}

func ExchangeBalanceForCredits(user model.AuthUser, amountCents int64) (model.BalanceExchangeResult, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.BalanceExchangeResult{}, err
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	if payment.CreditsPerYuan <= 0 {
		return model.BalanceExchangeResult{}, safeMessageError{message: "余额兑换暂未开放"}
	}
	if amountCents <= 0 || amountCents%100 != 0 {
		return model.BalanceExchangeResult{}, safeMessageError{message: "兑换金额必须是正整数元"}
	}
	yuan := amountCents / 100
	maxInt := int64(^uint(0) >> 1)
	if yuan > maxInt/int64(payment.CreditsPerYuan) {
		return model.BalanceExchangeResult{}, safeMessageError{message: "兑换金额过大"}
	}
	credits := int(yuan * int64(payment.CreditsPerYuan))
	exchangeID := newID("exchange")
	timestamp := now()
	extra, _ := json.Marshal(map[string]any{"balanceCents": amountCents, "credits": credits, "creditsPerYuan": payment.CreditsPerYuan})
	updated, err := repository.ExchangeUserBalanceForCredits(user.ID, amountCents, credits, timestamp,
		model.BalanceLog{ID: newID("balance"), OrganizationID: user.OrganizationID, Type: model.BalanceLogTypeCreditsExchange, RelatedID: exchangeID, Remark: "余额兑换算力", Extra: string(extra)},
		model.CreditLog{ID: newID("credit"), OrganizationID: user.OrganizationID, Type: model.CreditLogTypeBalanceExchange, RelatedID: exchangeID, Remark: "余额兑换算力", Extra: string(extra)},
	)
	if errors.Is(err, repository.ErrInsufficientBalance) {
		return model.BalanceExchangeResult{}, safeMessageError{message: "账户余额不足"}
	}
	if errors.Is(err, repository.ErrPaymentUserNotFound) {
		return model.BalanceExchangeResult{}, safeMessageError{message: "用户不存在"}
	}
	if err != nil {
		return model.BalanceExchangeResult{}, err
	}
	return model.BalanceExchangeResult{ExchangeID: exchangeID, BalanceCents: updated.BalanceCents, PersonalCredits: updated.Credits, SpentBalanceCents: amountCents, ReceivedCredits: credits}, nil
}

func HandlePaymentNotification(values url.Values) (model.PaymentOrder, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.PaymentOrder{}, err
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	params := paymentParameters(values)
	if payment.MerchantID == "" || payment.MerchantKey == "" || params["pid"] != payment.MerchantID || !strings.EqualFold(params["sign_type"], "MD5") {
		return model.PaymentOrder{}, errors.New("invalid payment notification")
	}
	expectedSign := signPaymentParameters(params, payment.MerchantKey)
	actualSign := strings.ToLower(strings.TrimSpace(params["sign"]))
	if len(actualSign) != len(expectedSign) || subtle.ConstantTimeCompare([]byte(actualSign), []byte(expectedSign)) != 1 {
		return model.PaymentOrder{}, errors.New("invalid payment signature")
	}
	if params["trade_status"] != "TRADE_SUCCESS" {
		return model.PaymentOrder{}, errors.New("payment not successful")
	}
	order, err := repository.GetPaymentOrderByOrderNo(params["out_trade_no"])
	if err != nil {
		return model.PaymentOrder{}, err
	}
	amount, err := parsePaymentAmount(params["money"])
	if err != nil || amount != order.AmountCents || order.BalanceCents <= 0 || params["type"] != string(order.Method) || strings.TrimSpace(params["trade_no"]) == "" {
		return model.PaymentOrder{}, errors.New("payment notification does not match order")
	}
	extra, _ := json.Marshal(map[string]any{"amountCents": order.AmountCents, "balanceCents": order.BalanceCents, "method": order.Method})
	order, _, err = repository.SettlePaymentOrder(order.OrderNo, params["trade_no"], now(), model.BalanceLog{
		ID: newID("balance"), Type: model.BalanceLogTypePaymentRecharge, Remark: "在线支付充值", Extra: string(extra),
	})
	return order, err
}

func paymentParameters(values url.Values) map[string]string {
	params := make(map[string]string, len(values))
	for key := range values {
		params[key] = strings.TrimSpace(values.Get(key))
	}
	return params
}

func signPaymentParameters(params map[string]string, merchantKey string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key != "sign" && key != "sign_type" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	// MD5 is required by the EasyPay V1 protocol; the merchant key is appended before hashing.
	sum := md5.Sum([]byte(strings.Join(parts, "&") + merchantKey))
	return hex.EncodeToString(sum[:])
}

func paymentMethodEnabled(methods []model.PaymentMethod, target model.PaymentMethod) bool {
	for _, method := range methods {
		if method == target {
			return true
		}
	}
	return false
}

func paymentPackageByID(packages []model.PaymentPackage, id string) (model.PaymentPackage, bool) {
	for _, item := range packages {
		if item.ID == id {
			return item, true
		}
	}
	return model.PaymentPackage{}, false
}

func paymentPublicBaseURL() (string, error) {
	value := strings.TrimRight(strings.TrimSpace(config.Cfg.PublicBaseURL), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", safeMessageError{message: "未配置有效的 PUBLIC_BASE_URL，无法接收支付结果"}
	}
	return value, nil
}

func paymentSubmitURL(gateway string) string {
	gateway = strings.TrimRight(strings.TrimSpace(gateway), "/")
	if strings.HasSuffix(strings.ToLower(gateway), "/submit.php") {
		return gateway
	}
	return gateway + "/submit.php"
}

func formatPaymentAmount(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func parsePaymentAmount(value string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid payment amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("invalid payment amount")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if strings.Trim(parts[0], "0123456789") != "" || strings.Trim(fraction, "0123456789") != "" || len(fraction) > 2 {
		return 0, errors.New("invalid payment amount")
	}
	fraction += strings.Repeat("0", 2-len(fraction))
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil || whole > (math.MaxInt64-cents)/100 {
		return 0, errors.New("invalid payment amount")
	}
	return whole*100 + cents, nil
}
