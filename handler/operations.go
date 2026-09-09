package handler

import (
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
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
