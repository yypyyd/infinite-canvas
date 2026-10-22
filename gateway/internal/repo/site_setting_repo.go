package repo

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"gorm.io/gorm"
)

const siteSettingCachePrefix = "setting:"

// siteSettingCacheTTL is a safety-net expiry; writes invalidate eagerly, so this
// only bounds staleness if an invalidation is ever missed (e.g. Redis blip).
const siteSettingCacheTTL = 5 * time.Minute

type SiteSettingRepository struct {
	db    *gorm.DB
	cache *redis.Client
}

// NewSiteSettingRepository wires the config KV store. cache may be nil, in which
// case the repository transparently falls back to DB-only access.
func NewSiteSettingRepository(db *gorm.DB, cache *redis.Client) *SiteSettingRepository {
	return &SiteSettingRepository{db: db, cache: cache}
}

func (r *SiteSettingRepository) cacheKey(key string) string {
	return siteSettingCachePrefix + key
}

func (r *SiteSettingRepository) GetValue(ctx context.Context, key string) (string, error) {
	if r.cache != nil {
		if v, err := r.cache.Get(ctx, r.cacheKey(key)).Result(); err == nil {
			return v, nil
		}
		// redis.Nil (miss) or any transient cache error -> fall through to DB.
	}

	value := ""
	var setting model.SiteSetting
	if err := r.db.WithContext(ctx).First(&setting, "key = ?", key).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return "", err
		}
		// Not found stays as "" — still cached below to absorb repeated misses.
	} else {
		value = setting.Value
	}

	if r.cache != nil {
		_ = r.cache.Set(ctx, r.cacheKey(key), value, siteSettingCacheTTL).Err()
	}
	return value, nil
}

func (r *SiteSettingRepository) UpsertValue(ctx context.Context, key, value string) error {
	if err := r.db.WithContext(ctx).Save(&model.SiteSetting{
		Key:   key,
		Value: value,
	}).Error; err != nil {
		return err
	}
	r.invalidate(ctx, key)
	return nil
}

func (r *SiteSettingRepository) UpsertValues(ctx context.Context, values map[string]string) error {
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			if err := tx.Save(&model.SiteSetting{
				Key:   key,
				Value: value,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for key := range values {
		r.invalidate(ctx, key)
	}
	return nil
}

// invalidate drops the cached entry so the next read repopulates from the DB.
// Deleting (rather than overwriting) keeps writes simple and race-tolerant.
func (r *SiteSettingRepository) invalidate(ctx context.Context, key string) {
	if r.cache == nil {
		return
	}
	_ = r.cache.Del(ctx, r.cacheKey(key)).Err()
}
