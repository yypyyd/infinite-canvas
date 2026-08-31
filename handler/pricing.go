package handler

import (
	"encoding/json"
	"net/http"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

type replaceUserPricingDiscountsRequest struct {
	Items []model.UserPricingDiscount `json:"items"`
}

type pricingItemResponse struct {
	Model          string  `json:"model"`
	Modality       string  `json:"modality"`
	Operation      string  `json:"operation"`
	Unit           string  `json:"unit"`
	ResolutionTier string  `json:"resolutionTier"`
	EffectiveRatio float64 `json:"effectiveRatio"`
	Source         string  `json:"source"`
}

type pricingResponse struct {
	Group      string                `json:"group"`
	GroupRatio float64               `json:"groupRatio"`
	Items      []pricingItemResponse `json:"items"`
}

func AdminUserPricingDiscounts(w http.ResponseWriter, _ *http.Request, userID string) {
	items, err := service.ListUserPricingDiscounts(userID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, items)
}

func AdminReplaceUserPricingDiscounts(w http.ResponseWriter, r *http.Request, userID string) {
	var request replaceUserPricingDiscountsRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Items == nil {
		Fail(w, "请求参数无效")
		return
	}
	items, err := service.ReplaceUserPricingDiscounts(userID, request.Items)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, items)
}

func Pricing(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	resolver, err := service.NewPricingResolver(user.ID, user.Group)
	if err != nil {
		FailError(w, err)
		return
	}
	results, err := resolver.ResolveAll()
	if err != nil {
		FailError(w, err)
		return
	}
	items := make([]pricingItemResponse, 0, len(results))
	for _, result := range results {
		items = append(items, pricingItemResponse{
			Model: result.Snapshot.Model, Modality: result.Snapshot.Modality,
			Operation: result.Snapshot.Operation, Unit: result.Snapshot.Unit,
			ResolutionTier: result.Snapshot.ResolutionTier,
			EffectiveRatio: result.EffectiveRatio, Source: string(result.Source),
		})
	}
	OK(w, pricingResponse{Group: resolver.Group(), GroupRatio: resolver.BaseGroupRatio(), Items: items})
}
