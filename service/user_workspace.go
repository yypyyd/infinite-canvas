package service

import (
	"encoding/json"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const maxWorkspaceRecordBytes = 16 << 20

type WorkspaceRecord struct {
	Domain    string          `json:"domain"`
	ObjectID  string          `json:"objectId"`
	Data      json.RawMessage `json:"data,omitempty"`
	Version   int64           `json:"version"`
	Deleted   bool            `json:"deleted"`
	UpdatedAt string          `json:"updatedAt"`
}

type WorkspacePayload struct {
	Records []WorkspaceRecord `json:"records"`
}

type WorkspaceChangeInput struct {
	Domain   string          `json:"domain"`
	ObjectID string          `json:"objectId"`
	Data     json.RawMessage `json:"data"`
	Deleted  bool            `json:"deleted"`
}

type WorkspaceChangeRequest struct {
	Changes []WorkspaceChangeInput `json:"changes"`
}

func UserWorkspace(userID string) (WorkspacePayload, error) {
	projects, err := repository.ListUserProjects(userID)
	if err != nil {
		return WorkspacePayload{}, err
	}
	assets, err := repository.ListUserAssets(userID)
	if err != nil {
		return WorkspacePayload{}, err
	}
	records := make([]WorkspaceRecord, 0, len(projects)+len(assets))
	for _, item := range projects {
		records = append(records, workspaceProjectRecord(item))
	}
	for _, item := range assets {
		records = append(records, workspaceAssetRecord(item))
	}
	return WorkspacePayload{Records: records}, nil
}

func ApplyUserWorkspaceChanges(userID string, request WorkspaceChangeRequest) (WorkspacePayload, error) {
	if len(request.Changes) > 200 {
		return WorkspacePayload{}, safeMessageError{message: "单次最多保存 200 条数据"}
	}
	mutations := make([]model.UserWorkspaceMutation, 0, len(request.Changes))
	for _, change := range request.Changes {
		if !validWorkspaceDomain(change.Domain) || strings.TrimSpace(change.ObjectID) == "" || len(change.ObjectID) > 160 {
			return WorkspacePayload{}, safeMessageError{message: "账号数据类型或编号无效"}
		}
		if !change.Deleted && (len(change.Data) == 0 || len(change.Data) > maxWorkspaceRecordBytes || !json.Valid(change.Data)) {
			return WorkspacePayload{}, safeMessageError{message: "账号数据为空、过大或格式不正确"}
		}
		data := change.Data
		if change.Deleted && len(data) == 0 {
			data = json.RawMessage(`{}`)
		}
		var summary struct {
			Title string `json:"title"`
			Kind  string `json:"kind"`
		}
		_ = json.Unmarshal(data, &summary)
		mutations = append(mutations, model.UserWorkspaceMutation{RecordID: newID("version"), Domain: change.Domain, ObjectID: change.ObjectID, Title: summary.Title, Kind: summary.Kind, Data: string(data), Deleted: change.Deleted})
	}
	projects, assets, err := repository.ApplyUserWorkspaceMutations(userID, mutations, now())
	if err == nil {
		state, _, _ := repository.GetUserWorkspaceState(userID)
		state.UserID = userID
		state.UpdatedAt = now()
		err = repository.SaveUserWorkspaceState(state)
	}
	if err == nil {
		_ = cleanupUserWorkspaceFiles(userID)
	}
	records := make([]WorkspaceRecord, 0, len(projects)+len(assets))
	for _, item := range projects {
		records = append(records, workspaceProjectRecord(item))
	}
	for _, item := range assets {
		records = append(records, workspaceAssetRecord(item))
	}
	return WorkspacePayload{Records: records}, err
}

func workspaceProjectRecord(item model.UserProject) WorkspaceRecord {
	return WorkspaceRecord{Domain: "canvas_project", ObjectID: item.ID, Data: validWorkspaceData(item.Data), Version: item.Version, Deleted: item.DeletedAt != "", UpdatedAt: item.UpdatedAt}
}

func validWorkspaceDomain(domain string) bool {
	return domain == "canvas_project" || domain == "asset"
}

func workspaceAssetRecord(item model.UserAsset) WorkspaceRecord {
	return WorkspaceRecord{Domain: "asset", ObjectID: item.ID, Data: validWorkspaceData(item.Data), Version: item.Version, Deleted: item.DeletedAt != "", UpdatedAt: item.UpdatedAt}
}

func validWorkspaceData(data string) json.RawMessage {
	value := json.RawMessage(data)
	if !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}
