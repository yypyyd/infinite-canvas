package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var promptCategories = []model.PromptCategory{
	{Category: "jeremy-product-photography", Name: "商品摄影灵感 · CC0", Description: "JeremyGDM 整理的商品主图、细节、场景、食品与营销海报提示词", GithubURL: "https://github.com/JeremyGDM/awesome-ai-product-photography-prompts", Remote: true},
	{Category: "joesai-commercial-prompts", Name: "商业商品图 · GPT Image 2", Description: "JoeSai 整理的 MIT 商业商品图、行业视觉与电商转化提示词", GithubURL: "https://github.com/JoeSai/awesome-gpt-image-2-commercial-prompts", Remote: true},
}

var (
	db     *gorm.DB
	dbOnce sync.Once
	dbErr  error
)

func transactionWithSQLiteRetry(db *gorm.DB, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := db.Transaction(fn)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "sqlite") || (!strings.Contains(strings.ToLower(err.Error()), "busy") && !strings.Contains(strings.ToLower(err.Error()), "locked")) || attempt >= 5 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 400 * time.Millisecond)
	}
}

// DB 初始化并返回全局数据库连接。
func DB() (*gorm.DB, error) {
	dbOnce.Do(func() {
		driver := strings.ToLower(strings.TrimSpace(config.Cfg.StorageDriver))
		if driver == "" {
			driver = "sqlite"
		}
		dsn := config.Cfg.DatabaseDSN
		if driver == "sqlite" && dsn != ":memory:" {
			_ = os.MkdirAll(filepath.Dir(dsn), 0755)
		}
		if isPostgresDriver(driver) {
			dbErr = ensurePostgresDatabase(dsn)
			if dbErr != nil {
				return
			}
		}
		if driver == "mysql" {
			dbErr = ensureMySQLDatabase(dsn)
			if dbErr != nil {
				return
			}
		}
		db, dbErr = gorm.Open(dialector(driver, dsn), &gorm.Config{})
		if dbErr != nil {
			return
		}
		dbErr = db.AutoMigrate(
			&model.User{},
			&model.UserPricingDiscount{},
			&model.UserAPIKey{},
			&model.Organization{},
			&model.OrganizationMember{},
			&model.OrganizationInvitation{},
			&model.OrganizationAuditLog{},
			&model.OrganizationEmailOutbox{},
			&model.Brand{},
			&model.Product{},
			&model.ProductSKU{},
			&model.ProductionTemplate{},
			&model.ProductionTemplateVersion{},
			&model.BatchProductionJob{},
			&model.BatchProductionTemplateSelection{},
			&model.BatchProductionItem{},
			&model.BatchProductionSnapshot{},
			&model.VideoProject{},
			&model.VideoProjectVersion{},
			&model.EmailVerification{},
			&model.CreditLog{},
			&model.PaymentOrder{},
			&model.BalanceLog{},
			&model.ReferralCommission{},
			&model.CheckIn{},
			&model.RedemptionCode{},
			&model.GenerationTask{},
			&model.AgentSession{},
			&model.AgentMessage{},
			&model.AgentRun{},
			&model.AgentStep{},
			&model.AgentEvent{},
			&model.AgentMemory{},
			&model.AgentRunSnapshot{},
			&model.AgentPlanStep{},
			&model.AgentFeedback{},
			&model.UserFile{},
			&model.UserFileReference{},
			&model.UserFileUploadReservation{},
			&model.UserFileUploadRateLimit{},
			&model.UserObjectDeletion{},
			&model.UserProject{},
			&model.UserAsset{},
			&model.UserGenerationRecord{},
			&model.UserProjectVersion{},
			&model.UserWorkspaceState{},
			&model.UserPreference{},
			&model.Prompt{},
			&model.Asset{},
			&model.Setting{},
		)
		if dbErr == nil {
			dbErr = dropLegacyWorkspaceUniqueIndexes(db)
		}
		if dbErr == nil {
			dbErr = backfillLegacyOrganizationData(db)
		}
		if dbErr == nil {
			dbErr = deleteOtherPromptCategories(db)
		}
	})
	return db, dbErr
}

func dropLegacyWorkspaceUniqueIndexes(db *gorm.DB) error {
	for _, item := range []struct {
		model any
		name  string
	}{
		{model: &model.UserAsset{}, name: "idx_user_asset"},
		{model: &model.UserGenerationRecord{}, name: "idx_user_generation_record"},
	} {
		if db.Migrator().HasIndex(item.model, item.name) {
			if err := db.Migrator().DropIndex(item.model, item.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func backfillLegacyOrganizationData(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var users []model.User
		if err := tx.Select("id, organization_id").Where("organization_id <> ''").Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if err := assignLegacyOrganizationData(tx, user.ID, user.OrganizationID); err != nil {
				return err
			}
		}
		return nil
	})
}

func assignLegacyOrganizationData(tx *gorm.DB, userID string, organizationID string) error {
	for _, item := range []any{
		&model.GenerationTask{},
		&model.UserFile{},
		&model.UserProject{},
		&model.UserAsset{},
		&model.UserGenerationRecord{},
		&model.UserProjectVersion{},
	} {
		if err := tx.Model(item).Where("(organization_id = '' OR organization_id IS NULL) AND user_id = ?", userID).Update("organization_id", organizationID).Error; err != nil {
			return err
		}
	}
	return nil
}

func dialector(driver string, dsn string) gorm.Dialector {
	switch driver {
	case "mysql":
		return gormmysql.Open(dsn)
	case "postgres", "postgresql":
		return postgres.Open(dsn)
	default:
		return sqlite.Open(dsn)
	}
}

func isPostgresDriver(driver string) bool {
	return driver == "postgres" || driver == "postgresql"
}

func ensureMySQLDatabase(dsn string) error {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(cfg.DBName)
	if target == "" {
		return nil
	}
	ctx := context.Background()
	targetDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	err = targetDB.PingContext(ctx)
	_ = targetDB.Close()
	if err == nil {
		return nil
	}
	if !isMySQLError(err, 1049) {
		return err
	}

	maintenance := cfg.Clone()
	maintenance.DBName = ""
	serverDB, err := sql.Open("mysql", maintenance.FormatDSN())
	if err != nil {
		return err
	}
	defer serverDB.Close()

	_, err = serverDB.ExecContext(ctx, "CREATE DATABASE "+quoteMySQLIdentifier(target)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	if isMySQLError(err, 1007) {
		return nil
	}
	return err
}

func ensurePostgresDatabase(dsn string) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(cfg.Database)
	if target == "" {
		return nil
	}
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err == nil {
		_ = conn.Close(ctx)
		return nil
	}
	if !isPostgresError(err, "3D000") {
		return err
	}

	maintenance := cfg.Copy()
	maintenance.Database = "postgres"
	if strings.EqualFold(target, "postgres") {
		maintenance.Database = "template1"
	}
	conn, err = pgx.ConnectConfig(ctx, maintenance)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{target}.Sanitize(), pgx.QueryExecModeExec)
	if isPostgresError(err, "42P04") {
		return nil
	}
	return err
}

func isMySQLError(err error, number uint16) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == number
}

func isPostgresError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) || isMySQLError(err, 1062) || isPostgresError(err, "23505") {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") || strings.Contains(message, "constraint failed: unique")
}

func quoteMySQLIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
