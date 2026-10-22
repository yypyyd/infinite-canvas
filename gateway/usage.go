package gateway

import (
	"context"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"gorm.io/gorm"
)

// PoolEvent is the pool's record of one generation, used by 幻图 to attribute
// its own credits (looked up by RequestID / event ID) to the serving account.
type PoolEvent struct {
	ID           string
	RequestID    string
	Kind         string
	Status       string
	Model        string
	Provider     string
	AccountID    string
	AccountEmail string
	TS           time.Time
}

const maxUsageEvents = 50000

func toPoolEvents(rows []model.EventLog) []PoolEvent {
	items := make([]PoolEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, PoolEvent{ID: row.ID, RequestID: row.RequestID, Kind: row.Kind, Status: row.Status,
			Model: row.Model, Provider: row.Provider, AccountID: row.AccountID, AccountEmail: row.AccountEmail, TS: row.TS})
	}
	return items
}

func (m *Module) builtinEvents(ctx context.Context) *gorm.DB {
	query := m.app.DB.WithContext(ctx).Model(&model.EventLog{}).
		Select("id", "request_id", "kind", "status", "model", "provider", "account_id", "account_email", "ts").
		Where("kind IN ?", []string{"image", "video", "text"})
	if m.app.Builtin != nil {
		query = query.Where("api_credential_id = ?", m.app.Builtin.ID)
	}
	return query
}

// UsageEvents lists 幻图-originated pool generations since the given time.
func (m *Module) UsageEvents(ctx context.Context, since time.Time) ([]PoolEvent, error) {
	var rows []model.EventLog
	err := m.builtinEvents(ctx).Where("ts >= ?", since).Order("ts DESC").Limit(maxUsageEvents).Find(&rows).Error
	return toPoolEvents(rows), err
}

// EventsFor returns the pool events matching 幻图 request IDs or pool event IDs
// (video tasks keep the pool job ID as their upstream task ID).
func (m *Module) EventsFor(ctx context.Context, requestIDs, eventIDs []string) ([]PoolEvent, error) {
	if len(requestIDs) == 0 && len(eventIDs) == 0 {
		return nil, nil
	}
	if requestIDs == nil {
		requestIDs = []string{""}
	}
	if eventIDs == nil {
		eventIDs = []string{""}
	}
	var rows []model.EventLog
	err := m.builtinEvents(ctx).Where("(request_id IN ? AND request_id <> '') OR id IN ?", requestIDs, eventIDs).Find(&rows).Error
	return toPoolEvents(rows), err
}
