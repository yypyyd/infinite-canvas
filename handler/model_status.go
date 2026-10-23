package handler

import (
	"net/http"

	"github.com/yypyyd/infinite-canvas/service"
)

// ModelStatus returns aggregate green/amber/red health for plaza models.
func ModelStatus(w http.ResponseWriter, r *http.Request) {
	status, err := service.PublicModelStatus()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, status)
}