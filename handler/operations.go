package handler

import (
	"encoding/json"
	"net/http"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func Liveness(w http.ResponseWriter, _ *http.Request) {
	OK(w, service.RuntimeHealth{Status: "ok", Database: "unchecked"})
}

func Readiness(w http.ResponseWriter, r *http.Request) {
	result, err := service.CheckReadiness(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, response{Code: 1, Data: result, Msg: "服务暂未就绪"})
		return
	}
	OK(w, result)
}

func AdminOperationsHealth(w http.ResponseWriter, r *http.Request) {
	result, err := service.GetOperationsHealth(r.Context())
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDataConsistency(w http.ResponseWriter, r *http.Request) {
	result, err := service.InspectDataConsistency(r.Context())
	if err != nil { FailError(w, err); return }
	OK(w, result)
}

func AdminRepairDataConsistency(w http.ResponseWriter, r *http.Request) {
	var input model.RepairDataConsistencyInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil { Fail(w, "请求参数无效"); return }
	result, err := service.RepairDataConsistencyIssue(r.Context(), input)
	if err != nil { FailError(w, err); return }
	OK(w, result)
}
