package handler

import (
	"encoding/json"
	"net/http"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func Assets(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListAssets(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminAssets(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListAssets(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminSaveAsset(w http.ResponseWriter, r *http.Request) {
	var item model.Asset
	_ = json.NewDecoder(r.Body).Decode(&item)
	result, err := service.SaveAsset(item)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDeleteAsset(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteAsset(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func AdminUploadAssetFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(80 << 20); err != nil {
		Fail(w, "读取上传文件失败")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		Fail(w, "请选择要上传的文件")
		return
	}
	result, err := service.SaveAssetFile(file, header)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AssetFile(w http.ResponseWriter, r *http.Request, name string) {
	path, mimeType, ok := service.AssetFilePath(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if mimeType != "" {
		w.Header().Set("Content-Type", mimeType)
	}
	http.ServeFile(w, r, path)
}
