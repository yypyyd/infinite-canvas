package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	paymentReconciliationInterval = 30 * time.Second
	paymentReconciliationWindow   = 72 * time.Hour
	maxPaymentQueryResponseSize   = 64 << 10
)

var paymentQueryClient = &http.Client{Timeout: 10 * time.Second}

type paymentQueryResponse struct {
	Code int               `json:"code"`
	Data *paymentQueryData `json:"data"`
}

type paymentQueryData struct {
	MerchantID json.RawMessage     `json:"id"`
	Method     model.PaymentMethod `json:"type"`
	TradeNo    string              `json:"trade_no"`
	OrderNo    string              `json:"out_trade_no"`
	Money      string              `json:"money"`
	Status     int                 `json:"status"`
}

func StartPaymentReconciliationWorker() {
	logWorkerInfo("payment_reconciliation", "worker_started", "interval_seconds", int(paymentReconciliationInterval/time.Second), "window_hours", int(paymentReconciliationWindow/time.Hour))
	go func() {
		for {
			reconcilePendingPaymentOrders(time.Now().UTC())
			time.Sleep(paymentReconciliationInterval)
		}
	}()
}

func reconcilePendingPaymentOrders(timestamp time.Time) {
	settings, err := repository.GetSettings()
	if err != nil {
		logWorkerError("payment_reconciliation", "settings_load_failed", err)
		return
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	if payment.MerchantID == "" || payment.MerchantKey == "" {
		return
	}
	orders, err := repository.ListPendingPaymentOrders(timestamp.Add(-paymentReconciliationWindow).Format(timestampLayout), 100)
	if err != nil {
		logWorkerError("payment_reconciliation", "pending_list_failed", err)
		return
	}
	for _, order := range orders {
		settledOrder, settled, reconcileErr := reconcilePaymentOrderWithSetting(order, payment)
		if reconcileErr != nil {
			logWorkerError("payment_reconciliation", "order_query_failed", reconcileErr, "order_id", order.ID)
			continue
		}
		if settled {
			logWorkerInfo("payment_reconciliation", "order_settled", "order_id", settledOrder.ID, "method", settledOrder.Method, "amount_cents", settledOrder.AmountCents, "balance_cents", settledOrder.BalanceCents)
		}
	}
}

func reconcilePaymentOrder(order model.PaymentOrder) (model.PaymentOrder, bool, error) {
	if order.Status != model.PaymentOrderStatusPending {
		return order, false, nil
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return order, false, err
	}
	payment := normalizePaymentSetting(settings.Private.Payment)
	if payment.MerchantID == "" || payment.MerchantKey == "" {
		return order, false, nil
	}
	return reconcilePaymentOrderWithSetting(order, payment)
}

func reconcilePaymentOrderWithSetting(order model.PaymentOrder, payment model.PaymentSetting) (model.PaymentOrder, bool, error) {
	queryURL, err := paymentOrderQueryURL(payment.GatewayURL)
	if err != nil {
		return order, false, err
	}
	paid, tradeNo, err := queryEasyPayOrder(paymentQueryClient, queryURL, payment, order)
	if err != nil || !paid {
		return order, false, err
	}
	extra, _ := json.Marshal(map[string]any{
		"amountCents": order.AmountCents, "balanceCents": order.BalanceCents, "method": order.Method, "settlementSource": "order_query",
	})
	return repository.SettlePaymentOrder(order.OrderNo, tradeNo, now(), model.BalanceLog{
		ID: newID("balance"), Type: model.BalanceLogTypePaymentRecharge, Remark: "在线支付充值", Extra: string(extra),
	})
}

func queryEasyPayOrder(client *http.Client, queryURL string, payment model.PaymentSetting, order model.PaymentOrder) (bool, string, error) {
	form := url.Values{"order_no": []string{order.OrderNo}, "type": []string{"2"}}
	request, err := http.NewRequest(http.MethodPost, queryURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return false, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, "", errors.New("payment query returned a non-success HTTP status")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPaymentQueryResponseSize+1))
	if err != nil {
		return false, "", err
	}
	if len(body) > maxPaymentQueryResponseSize {
		return false, "", errors.New("payment query response is too large")
	}
	var result paymentQueryResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return false, "", errors.New("payment query response is invalid")
	}
	if result.Code != http.StatusOK || result.Data == nil {
		return false, "", nil
	}
	return validatePaymentQueryResult(payment, order, *result.Data)
}

func validatePaymentQueryResult(payment model.PaymentSetting, order model.PaymentOrder, result paymentQueryData) (bool, string, error) {
	merchantID, err := paymentQueryMerchantID(result.MerchantID)
	if err != nil || merchantID != payment.MerchantID || strings.TrimSpace(result.OrderNo) != order.OrderNo || result.Method != order.Method {
		return false, "", errors.New("payment query does not match order")
	}
	amount, err := parsePaymentAmount(result.Money)
	if err != nil || amount != order.AmountCents || order.BalanceCents <= 0 {
		return false, "", errors.New("payment query amount does not match order")
	}
	if result.Status != 0 && result.Status != 1 {
		return false, "", errors.New("payment query returned an unknown status")
	}
	if result.Status == 0 {
		return false, "", nil
	}
	tradeNo := strings.TrimSpace(result.TradeNo)
	if tradeNo == "" {
		return false, "", errors.New("paid payment query has no trade number")
	}
	return true, tradeNo, nil
}

func paymentQueryMerchantID(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value), nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	if _, err := strconv.ParseInt(number.String(), 10, 64); err != nil {
		return "", err
	}
	return number.String(), nil
}

func paymentOrderQueryURL(gateway string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(gateway))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("payment query gateway is invalid")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(basePath), "/submit.php") {
		basePath = basePath[:len(basePath)-len("/submit.php")]
	}
	parsed.Path = basePath + "/api/findorder"
	parsed.RawPath = ""
	return parsed.String(), nil
}
