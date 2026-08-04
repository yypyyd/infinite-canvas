package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
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
	Version  int64           `json:"version"`
}

type WorkspaceChangeRequest struct {
	Changes []WorkspaceChangeInput `json:"changes"`
}

func UserWorkspace(user model.AuthUser) (WorkspacePayload, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return WorkspacePayload{}, err }
	organizationID := organization.ID
	projects, err := repository.ListUserProjects(organizationID)
	if err != nil {
		return WorkspacePayload{}, err
	}
	assets, err := repository.ListUserAssets(organizationID)
	if err != nil {
		return WorkspacePayload{}, err
	}
	generationRecords, err := repository.ListUserGenerationRecords(organizationID)
	if err != nil {
		return WorkspacePayload{}, err
	}
	records := make([]WorkspaceRecord, 0, len(projects)+len(assets)+len(generationRecords))
	for _, item := range projects {
		records = append(records, workspaceProjectRecord(item))
	}
	for _, item := range assets {
		records = append(records, workspaceAssetRecord(item))
	}
	for _, item := range generationRecords {
		records = append(records, workspaceGenerationRecord(item))
	}
	return WorkspacePayload{Records: records}, nil
}

func ApplyUserWorkspaceChanges(user model.AuthUser, request WorkspaceChangeRequest) (WorkspacePayload, error) {
	if err := RequireOrganizationWrite(user); err != nil { return WorkspacePayload{}, err }
	organizationID := user.OrganizationID
	if len(request.Changes) > 200 {
		return WorkspacePayload{}, safeMessageError{message: "单次最多保存 200 条数据"}
	}
	mutations := make([]model.UserWorkspaceMutation, 0, len(request.Changes))
	for _, change := range request.Changes {
		if !validWorkspaceDomain(change.Domain) || strings.TrimSpace(change.ObjectID) == "" || len(change.ObjectID) > 160 || change.Version < 0 {
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
		if change.Domain == "generation_record" && !change.Deleted && summary.Kind != "image" && summary.Kind != "video" {
			return WorkspacePayload{}, safeMessageError{message: "生成记录类型无效"}
		}
		storageKeys := make(map[string]bool)
		if !change.Deleted { collectUserStorageKeys(data, storageKeys) }
		keys := make([]string, 0, len(storageKeys))
		for key := range storageKeys { keys = append(keys, key) }
		mutations = append(mutations, model.UserWorkspaceMutation{RecordID: newID("version"), Domain: change.Domain, ObjectID: change.ObjectID, Title: summary.Title, Kind: summary.Kind, Data: string(data), Deleted: change.Deleted, ExpectedVersion: change.Version, StorageKeys: keys})
	}
	timestamp := now()
	projects, assets, generationRecords, err := repository.ApplyUserWorkspaceMutations(organizationID, user.ID, mutations, timestamp, newAuditLog(user.ID, organizationID, "workspace.save", "workspace", organizationID, len(request.Changes), timestamp))
	if errors.Is(err, repository.ErrWorkspaceVersionConflict) { return WorkspacePayload{}, safeMessageError{message: "企业数据已被其他成员更新，请刷新后重新编辑"} }
	if errors.Is(err, repository.ErrWorkspaceFileMissing) { return WorkspacePayload{}, safeMessageError{message: "企业媒体文件不存在，请重新上传后保存"} }
	if err == nil {
		_ = cleanupUserWorkspaceFiles(organizationID)
	}
	records := make([]WorkspaceRecord, 0, len(projects)+len(assets)+len(generationRecords))
	for _, item := range projects {
		records = append(records, workspaceProjectRecord(item))
	}
	for _, item := range assets {
		records = append(records, workspaceAssetRecord(item))
	}
	for _, item := range generationRecords {
		records = append(records, workspaceGenerationRecord(item))
	}
	return WorkspacePayload{Records: records}, err
}

func workspaceProjectRecord(item model.UserProject) WorkspaceRecord {
	return WorkspaceRecord{Domain: "canvas_project", ObjectID: item.ID, Data: validWorkspaceData(item.Data), Version: item.Version, Deleted: item.DeletedAt != "", UpdatedAt: item.UpdatedAt}
}

func validWorkspaceDomain(domain string) bool {
	return domain == "canvas_project" || domain == "asset" || domain == "generation_record"
}

func workspaceAssetRecord(item model.UserAsset) WorkspaceRecord {
	return WorkspaceRecord{Domain: "asset", ObjectID: item.ID, Data: validWorkspaceData(item.Data), Version: item.Version, Deleted: item.DeletedAt != "", UpdatedAt: item.UpdatedAt}
}

func workspaceGenerationRecord(item model.UserGenerationRecord) WorkspaceRecord {
	return WorkspaceRecord{Domain: "generation_record", ObjectID: item.ID, Data: validWorkspaceData(item.Data), Version: item.Version, Deleted: item.DeletedAt != "", UpdatedAt: item.UpdatedAt}
}

func validWorkspaceData(data string) json.RawMessage {
	value := json.RawMessage(data)
	if !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}
