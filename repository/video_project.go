package repository

import (
	"encoding/json"
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListVideoProjects(organizationID string, q model.Query) ([]model.VideoProject, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.VideoProject{}).Where("organization_id = ?", organizationID)
	if q.Keyword != "" { tx = tx.Where("name LIKE ? OR description LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.VideoProject
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func GetVideoProject(organizationID, id string) (model.VideoProject, bool, error) {
	db, err := DB()
	if err != nil { return model.VideoProject{}, false, err }
	var item model.VideoProject
	err = db.Where("organization_id = ? AND id = ?", organizationID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.VideoProject{}, false, nil }
	return item, err == nil, err
}

func SaveVideoProject(item model.VideoProject, expectedVersion int64, storageKeys []string, auditLogs ...model.OrganizationAuditLog) (model.VideoProject, error) {
	db, err := DB()
	if err != nil { return item, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		if expectedVersion == 0 {
			if err := tx.Create(&item).Error; err != nil { return err }
		} else {
			var saved model.VideoProject
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).First(&saved).Error; err != nil { return err }
			if saved.Version != expectedVersion { return ErrCommerceVersionConflict }
			item.CreatedAt, item.CreatedBy, item.CurrentVersion = saved.CreatedAt, saved.CreatedBy, saved.CurrentVersion
			result := tx.Model(&model.VideoProject{}).Where("organization_id = ? AND id = ? AND version = ?", item.OrganizationID, item.ID, expectedVersion).Updates(map[string]any{"name": item.Name, "description": item.Description, "product_id": item.ProductID, "sku_id": item.SKUID, "draft_timeline_json": item.DraftTimelineJSON, "status": item.Status, "version": item.Version, "updated_at": item.UpdatedAt})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		}
		if err := replaceUserFileReferences(tx, item.OrganizationID, "video_project_draft", item.ID, "video-project-draft-"+item.ID, storageKeys, false, item.UpdatedAt); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func ListVideoProjectVersions(organizationID, projectID string) ([]model.VideoProjectVersion, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.VideoProjectVersion
	err = db.Where("organization_id = ? AND project_id = ?", organizationID, projectID).Order("version desc").Find(&items).Error
	if err != nil { return nil, err }
	for index := range items {
		if err := json.Unmarshal([]byte(items[index].TimelineJSON), &items[index].Timeline); err != nil { return nil, err }
		if err := json.Unmarshal([]byte(items[index].OutputSpecJSON), &items[index].OutputSpec); err != nil { return nil, err }
	}
	return items, nil
}

func GetVideoProjectVersion(organizationID, projectID string, versionNumber int) (model.VideoProjectVersion, bool, error) {
	db, err := DB()
	if err != nil { return model.VideoProjectVersion{}, false, err }
	var item model.VideoProjectVersion
	err = db.Where("organization_id = ? AND project_id = ? AND version = ?", organizationID, projectID, versionNumber).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.VideoProjectVersion{}, false, nil }
	if err != nil { return item, false, err }
	if err := json.Unmarshal([]byte(item.TimelineJSON), &item.Timeline); err != nil { return item, false, err }
	if err := json.Unmarshal([]byte(item.OutputSpecJSON), &item.OutputSpec); err != nil { return item, false, err }
	return item, true, nil
}

func CreateVideoProjectVersion(project model.VideoProject, expectedTimelineJSON string, version model.VideoProjectVersion, storageKeys []string, auditLogs ...model.OrganizationAuditLog) (model.VideoProjectVersion, error) {
	db, err := DB()
	if err != nil { return version, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var saved model.VideoProject
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", project.OrganizationID, project.ID).First(&saved).Error; err != nil { return err }
		if saved.Version != project.Version || saved.DraftTimelineJSON != expectedTimelineJSON { return ErrCommerceVersionConflict }
		version.Version = saved.CurrentVersion + 1
		version.TimelineJSON = saved.DraftTimelineJSON
		var timeline model.VideoTimeline
		if err := json.Unmarshal([]byte(saved.DraftTimelineJSON), &timeline); err != nil { return err }
		outputSpecJSON, err := json.Marshal(timeline.Output)
		if err != nil { return err }
		version.Timeline, version.OutputSpec, version.OutputSpecJSON = timeline, timeline.Output, string(outputSpecJSON)
		if err := tx.Create(&version).Error; err != nil { return err }
		result := tx.Model(&model.VideoProject{}).Where("organization_id = ? AND id = ? AND version = ?", project.OrganizationID, project.ID, project.Version).Updates(map[string]any{"status": model.VideoProjectStatusVersioned, "current_version": version.Version, "version": project.Version + 1, "updated_at": version.CreatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		project.Status, project.CurrentVersion, project.Version, project.UpdatedAt = model.VideoProjectStatusVersioned, version.Version, project.Version+1, version.CreatedAt
		if err := replaceUserFileReferences(tx, project.OrganizationID, "video_project_version", version.ID, "video-project-version-"+version.ID, storageKeys, false, version.CreatedAt); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return version, err
}
