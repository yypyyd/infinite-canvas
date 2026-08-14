package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

type createPaymentOrderRequest struct {
	PackageID string              `json:"packageId"`
	Method    model.PaymentMethod `json:"method"`
}

type exchangeBalanceRequest struct {
	AmountCents int64 `json:"amountCents"`
}

func PaymentConfig(w http.ResponseWriter, _ *http.Request) {
	result, err := service.GetPaymentConfig()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func CreatePaymentOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var request createPaymentOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求参数无效")
		return
	}
	result, err := service.CreatePaymentOrder(user, request.PackageID, request.Method)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PaymentOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	result, err := service.ListPaymentOrders(user.ID, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PaymentOrder(w http.ResponseWriter, r *http.Request, orderNo string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	result, err := service.GetPaymentOrder(user.ID, orderNo)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func ExchangeBalance(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var request exchangeBalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求参数无效")
		return
	}
	result, err := service.ExchangeBalanceForCredits(user, request.AmountCents)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func PaymentNotify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := service.HandlePaymentNotification(r.URL.Query()); err != nil {
		_, _ = w.Write([]byte("fail"))
		return
	}
	_, _ = w.Write([]byte("success"))
}

func PaymentReturn(w http.ResponseWriter, r *http.Request) {
	state := "pending"
	orderNo := strings.TrimSpace(r.URL.Query().Get("out_trade_no"))
	if order, err := service.HandlePaymentNotification(r.URL.Query()); err == nil {
		state = "success"
		orderNo = order.OrderNo
	}
	query := url.Values{"tab": []string{"balance"}, "payment": []string{state}}
	if orderNo != "" {
		query.Set("orderNo", orderNo)
	}
	http.Redirect(w, r, "/account?"+query.Encode(), http.StatusFound)
}
