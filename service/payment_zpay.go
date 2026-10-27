package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
)

var zpayHTTPClient = &http.Client{
	Timeout: 3 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func verifyZPayOrder(ctx context.Context, payment model.PaymentSetting, order model.PaymentOrder, tradeNo string) error {
	submitURL, err := url.Parse(paymentSubmitURL(payment.GatewayURL))
	if err != nil {
		return errors.New("invalid ZPay gateway")
	}
	queryURL, err := submitURL.Parse("api.php")
	if err != nil {
		return errors.New("invalid ZPay order query URL")
	}
	queryURL.RawQuery = url.Values{
		"act": {"order"}, "pid": {payment.MerchantID}, "key": {payment.MerchantKey}, "out_trade_no": {order.OrderNo},
	}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL.String(), nil)
	if err != nil {
		return errors.New("could not create ZPay order query")
	}
	response, err := zpayHTTPClient.Do(request)
	if err != nil {
		// The request URL contains the merchant key; never return or log it.
		return errors.New("ZPay order query failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("ZPay order query returned an error")
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return errors.New("invalid ZPay order query response")
	}
	amount, err := parsePaymentAmount(fmt.Sprint(result["money"]))
	if err != nil || fmt.Sprint(result["code"]) != "1" || fmt.Sprint(result["status"]) != "1" ||
		fmt.Sprint(result["pid"]) != payment.MerchantID || fmt.Sprint(result["out_trade_no"]) != order.OrderNo ||
		fmt.Sprint(result["trade_no"]) != tradeNo || fmt.Sprint(result["type"]) != string(order.Method) || amount != order.AmountCents {
		return errors.New("ZPay paid order verification failed")
	}
	return nil
}
