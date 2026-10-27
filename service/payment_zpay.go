package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
)

var zpayHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func createZPayCheckout(ctx context.Context, payment model.PaymentSetting, order model.PaymentOrder, params map[string]string) (model.PaymentSubmission, error) {
	params["sign"] = signPaymentParameters(params, payment.MerchantKey)
	params["sign_type"] = "MD5"
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range params {
		if err := writer.WriteField(key, value); err != nil {
			return model.PaymentSubmission{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return model.PaymentSubmission{}, err
	}
	endpoint := strings.TrimSuffix(paymentSubmitURL(payment.GatewayURL), "/submit.php") + "/mapi.php"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return model.PaymentSubmission{}, safeMessageError{message: "ZPay 网关地址无效"}
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := zpayHTTPClient.Do(request)
	if err != nil {
		return model.PaymentSubmission{}, safeMessageError{message: "ZPay 下单连接失败，请稍后重试"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return model.PaymentSubmission{}, safeMessageError{message: "ZPay 下单失败，请稍后重试"}
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil || fmt.Sprint(result["code"]) != "1" {
		return model.PaymentSubmission{}, safeMessageError{message: "ZPay 下单失败，请核对商户信息及支付渠道"}
	}
	qrCode, _ := result["qrcode"].(string)
	qrImageURL, _ := result["img"].(string)
	payURL, _ := result["payurl"].(string)
	checkout := &model.PaymentCheckout{QRCode: strings.TrimSpace(qrCode), QRImageURL: zpayHTTPSURL(qrImageURL), PayURL: zpayHTTPSURL(payURL)}
	if checkout.QRCode == "" && checkout.QRImageURL == "" {
		checkout.QRCode = checkout.PayURL
	}
	if checkout.QRCode == "" && checkout.QRImageURL == "" {
		return model.PaymentSubmission{}, safeMessageError{message: "ZPay 未返回可用的支付二维码"}
	}
	return model.PaymentSubmission{Order: order, Checkout: checkout}, nil
}

func zpayHTTPSURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	return parsed.String()
}

func verifyZPayOrder(ctx context.Context, payment model.PaymentSetting, order model.PaymentOrder, tradeNo string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
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
