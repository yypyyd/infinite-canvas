package handler

import (
	"encoding/json"
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
)

type generateRedemptionCodesRequest struct {
	Credits  int    `json:"credits"`
	Quantity int    `json:"quantity"`
	Prefix   string `json:"prefix"`
	Remark   string `json:"remark"`
}

type redeemCodeRequest struct {
	Code string `json:"code"`
}

type deleteRedemptionCodesRequest struct {
	IDs    []string `json:"ids"`
	Status string   `json:"status"`
}

func AdminRedemptionCodes(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListRedemptionCodes(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminGenerateRedemptionCodes(w http.ResponseWriter, r *http.Request) {
	var request generateRedemptionCodesRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.GenerateRedemptionCodes(request.Credits, request.Quantity, request.Prefix, request.Remark)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDeleteRedemptionCode(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteRedemptionCode(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func AdminDeleteRedemptionCodes(w http.ResponseWriter, r *http.Request) {
	var request deleteRedemptionCodesRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	deleted, err := service.DeleteRedemptionCodes(request.IDs, request.Status)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, deleted)
}

func RedeemCode(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var request redeemCodeRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	result, err := service.RedeemCode(user.ID, request.Code)
	if err != nil {
		FailError(w, err)
		return
	}
	result.OrganizationID = user.OrganizationID
	result, err = service.ApplyEffectiveCredits(result)
	if err != nil { FailError(w, err); return }
	OK(w, result)
}
