package repository

import "github.com/yypyyd/infinite-canvas/model"

func ListManagedAgentMemories(organizationID, userID, projectID string, query model.Query) ([]model.AgentMemory, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	query.Normalize()
	base := db.Model(&model.AgentMemory{}).Where("organization_id = ? AND user_id = ? AND (project_id = ? OR project_id = '') AND status = ?", organizationID, userID, projectID, model.AgentMemoryStatusActive)
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.AgentMemory{}
	err = base.Order("updated_at desc, id asc").Offset(query.Offset()).Limit(query.PageSize).Find(&items).Error
	return items, total, err
}

func GetOwnedAgentMemory(organizationID, userID, id string) (model.AgentMemory, error) {
	db, err := DB()
	if err != nil {
		return model.AgentMemory{}, err
	}
	var item model.AgentMemory
	err = db.Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, id, model.AgentMemoryStatusActive).First(&item).Error
	return item, err
}

func UpdateOwnedAgentMemory(organizationID, userID, id, content, timestamp string, forget bool) error {
	db, err := DB()
	if err != nil {
		return err
	}
	updates := map[string]any{"content": content, "confidence": 1, "updated_at": timestamp}
	if forget {
		updates = map[string]any{"status": model.AgentMemoryStatusForgotten, "forgotten_at": timestamp, "updated_at": timestamp}
	}
	return db.Model(&model.AgentMemory{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, id, model.AgentMemoryStatusActive).Updates(updates).Error
}
