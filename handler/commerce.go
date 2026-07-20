package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func CommerceWorkspace(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.OrganizationWorkspace(user) })
}

func CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var input struct { Name string `json:"name"` }
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.CreateOrganization(user, input.Name) })
}

func SwitchOrganization(w http.ResponseWriter, r *http.Request) {
	var input struct { OrganizationID string `json:"organizationId"` }
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.SwitchOrganization(user, input.OrganizationID) })
}

func UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	var input struct { Name string `json:"name"` }
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.UpdateOrganization(user, input.Name) })
}

func InviteOrganizationMember(w http.ResponseWriter, r *http.Request) {
	var input struct { Email string `json:"email"`; Role model.OrganizationRole `json:"role"` }
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.InviteOrganizationMember(user, input.Email, input.Role) })
}

func PendingOrganizationInvitations(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListPendingOrganizationInvitations(user) })
}

func CurrentOrganizationInvitations(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListCurrentOrganizationInvitations(user) })
}

func AcceptOrganizationInvitation(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.AcceptOrganizationInvitation(user, id) })
}

func RevokeOrganizationInvitation(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.RevokeOrganizationInvitation(user, id) })
}

func UpdateOrganizationMember(w http.ResponseWriter, r *http.Request, id string) {
	var input struct { Role model.OrganizationRole `json:"role"` }
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.UpdateOrganizationMember(user, id, input.Role) })
}

func CommerceOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListOrganizationMembers(user, parseQuery(r)) })
}

func RemoveOrganizationMember(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.RemoveOrganizationMember(user, id) })
}

func TransferOrganizationOwnership(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.TransferOrganizationOwnership(user, id) })
}

func CommerceBrands(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListBrands(user, parseQuery(r)) })
}

func SaveCommerceBrand(w http.ResponseWriter, r *http.Request) {
	var item model.Brand
	if !decodeCommerceJSON(w, r, &item) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.SaveBrand(user, item) })
}

func DeleteCommerceBrand(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.DeleteBrand(user, id) })
}

func CommerceProducts(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListProducts(user, parseQuery(r)) })
}

func SaveCommerceProduct(w http.ResponseWriter, r *http.Request) {
	var item model.Product
	if !decodeCommerceJSON(w, r, &item) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.SaveProduct(user, item) })
}

func DeleteCommerceProduct(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.DeleteProduct(user, id) })
}

func CommerceProductSKUs(w http.ResponseWriter, r *http.Request, productID string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListProductSKUs(user, productID, parseQuery(r)) })
}

func SaveCommerceProductSKU(w http.ResponseWriter, r *http.Request) {
	var item model.ProductSKU
	if !decodeCommerceJSON(w, r, &item) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.SaveProductSKU(user, item) })
}

func DeleteCommerceProductSKU(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.DeleteProductSKU(user, id) })
}

func CommerceBatchJobs(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListBatchProductionJobs(user, parseQuery(r)) })
}

func CreateCommerceBatchJob(w http.ResponseWriter, r *http.Request) {
	var input model.CreateBatchProductionJobInput
	if !decodeCommerceJSON(w, r, &input) { return }
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.CreateBatchProductionJob(user, input) })
}

func CommerceBatchItems(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListBatchProductionItems(user, id, parseQuery(r)) })
}

func CancelCommerceBatchJob(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.CancelBatchProductionJob(user, id) })
}

func RetryCommerceBatchJob(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return true, service.RetryBatchProductionJob(user, id) })
}

func CommerceAuditLogs(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) {
		items, total, err := service.ListOrganizationAuditLogs(user, parseQuery(r))
		return map[string]any{"items": items, "total": total}, err
	})
}

func withCommerceUser(w http.ResponseWriter, r *http.Request, action func(model.AuthUser) (any, error)) {
	user, ok := service.UserFromContext(r.Context())
	if !ok { Fail(w, "未登录或权限不足"); return }
	result, err := action(user)
	if err != nil { FailError(w, err); return }
	OK(w, result)
}

func decodeCommerceJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil { Fail(w, "请求参数无效"); return false }
	if err := decoder.Decode(&struct{}{}); err != io.EOF { Fail(w, "请求参数无效"); return false }
	return true
}
