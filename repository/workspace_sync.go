package repository

import (
	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

func ListUserWorkspaceChanges(organizationID string, versions map[string]int64) ([]model.UserProject, []model.UserAsset, []model.UserGenerationRecord, error) {
	db, err := DB()
	if err != nil {
		return nil, nil, nil, err
	}
	projects, err := listWorkspaceChanges[model.UserProject](db, organizationID, "canvas_project", versions)
	if err != nil {
		return nil, nil, nil, err
	}
	assets, err := listWorkspaceChanges[model.UserAsset](db, organizationID, "asset", versions)
	if err != nil {
		return nil, nil, nil, err
	}
	records, err := listWorkspaceChanges[model.UserGenerationRecord](db, organizationID, "generation_record", versions)
	return projects, assets, records, err
}

func listWorkspaceChanges[T any](db *gorm.DB, organizationID, domain string, versions map[string]int64) ([]T, error) {
	var summaries []struct {
		ID      string
		Version int64
	}
	if err := db.Model(new(T)).Select("id, version").Where("organization_id = ?", organizationID).Find(&summaries).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, item := range summaries {
		if version, exists := versions[domain+":"+item.ID]; !exists || version != item.Version {
			ids = append(ids, item.ID)
		}
	}
	var items []T
	for start := 0; start < len(ids); start += 200 {
		var batch []T
		if err := db.Where("organization_id = ? AND id IN ?", organizationID, ids[start:min(start+200, len(ids))]).Find(&batch).Error; err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}
	return items, nil
}
