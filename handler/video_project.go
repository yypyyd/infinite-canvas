package handler

import (
	"net/http"
	"strconv"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func CommerceVideoProjects(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListVideoProjects(user, parseQuery(r)) })
}

func CreateCommerceVideoProject(w http.ResponseWriter, r *http.Request) {
	var input model.SaveVideoProjectInput
	if !decodeCommerceJSON(w, r, &input) {
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.SaveVideoProject(user, "", input) })
}

func CommerceVideoProject(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.GetVideoProject(user, id) })
}

func SaveCommerceVideoProject(w http.ResponseWriter, r *http.Request, id string) {
	var input model.SaveVideoProjectInput
	if !decodeCommerceJSON(w, r, &input) {
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.SaveVideoProject(user, id, input) })
}

func PreflightCommerceVideoProject(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.PreflightVideoProject(user, id) })
}

func CommerceVideoProjectVersions(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListVideoProjectVersions(user, id) })
}

func CommerceVideoProjectVersion(w http.ResponseWriter, r *http.Request, id, versionValue string) {
	versionNumber, err := strconv.Atoi(versionValue)
	if err != nil || versionNumber <= 0 {
		Fail(w, "视频工程版本无效")
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.GetVideoProjectVersion(user, id, versionNumber) })
}

func CreateCommerceVideoProjectVersion(w http.ResponseWriter, r *http.Request, id string) {
	var input model.CreateVideoProjectVersionInput
	if !decodeCommerceJSON(w, r, &input) {
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) {
		return service.CreateVideoProjectVersion(user, id, input.ExpectedVersion)
	})
}

func CommerceVideoRenderJobs(w http.ResponseWriter, r *http.Request) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ListVideoRenderJobs(user, parseQuery(r)) })
}

func CreateCommerceVideoRenderJob(w http.ResponseWriter, r *http.Request, projectID string) {
	var input model.CreateVideoRenderJobInput
	if !decodeCommerceJSON(w, r, &input) {
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.CreateVideoRenderJob(user, projectID, input) })
}

func CommerceVideoRenderJob(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.GetVideoRenderJob(user, id) })
}

func CancelCommerceVideoRenderJob(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) {
		if err := service.CancelVideoRenderJob(user, id); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})
}

func ReviewCommerceVideoRenderJob(w http.ResponseWriter, r *http.Request, id string) {
	var input model.ReviewVideoRenderJobInput
	if !decodeCommerceJSON(w, r, &input) {
		return
	}
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) { return service.ReviewVideoRenderJob(user, id, input) })
}

func DownloadCommerceVideoRenderJob(w http.ResponseWriter, r *http.Request, id string) {
	withCommerceUser(w, r, func(user model.AuthUser) (any, error) {
		url, err := service.VideoRenderResultURL(user, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"url": url}, nil
	})
}
