package repository

import (
	"errors"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxUserNumber = 999999999

func searchUserNumber(keyword string) int64 {
	n, err := strconv.ParseUint(keyword, 10, 32)
	if err != nil || n > maxUserNumber {
		return -1
	}
	return int64(n)
}

func assignUserNumber(tx *gorm.DB, user *model.User) error {
	result := tx.Model(&model.UserNumberSequence{}).Where("id = 1 AND last_number < ?", maxUserNumber).UpdateColumn("last_number", gorm.Expr("last_number + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("用户编号已用完")
	}
	var seq model.UserNumberSequence
	if err := tx.First(&seq, 1).Error; err != nil {
		return err
	}
	user.PublicID = &seq.LastNumber
	return nil
}

func initializeUserNumbers(db *gorm.DB) error {
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		seq := model.UserNumberSequence{ID: 1, LastNumber: -1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seq).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&seq, 1).Error; err != nil {
			return err
		}
		var users []model.User
		if err := tx.Select("id").Where("public_id IS NULL").Order("created_at, id").Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if err := assignUserNumber(tx, &user); err != nil {
				return err
			}
			if err := tx.Model(&user).UpdateColumn("public_id", *user.PublicID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListUsers 分页查询用户。
func ListUsers(q model.Query) ([]model.User, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.User{})
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("public_id = ? OR id LIKE ? OR username LIKE ? OR display_name LIKE ? OR email LIKE ?", searchUserNumber(keyword), like, like, like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []model.User
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&users).Error
	return users, total, err
}

// CountUsers 返回用户总数。
func CountUsers() (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	return total, db.Model(&model.User{}).Count(&total).Error
}

// HasAdmin 判断系统中是否存在管理员。
func HasAdmin() (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var total int64
	err = db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&total).Error
	return total > 0, err
}

// GetUserByID 根据 ID 查询用户。
func GetUserByID(id string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "id = ?", id)
}

// GetUserByUsername 根据用户名查询用户。
func GetUserByUsername(username string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "username = ?", username)
}

// SaveUser 保存用户信息。
func SaveUser(user model.User) (model.User, error) {
	db, err := DB()
	if err != nil {
		return user, err
	}
	expectedVersion := user.TokenVersion
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var current model.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", user.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && current.TokenVersion != expectedVersion {
			return gorm.ErrInvalidData
		}
		user.TokenVersion = current.TokenVersion
		if errors.Is(err, gorm.ErrRecordNotFound) || current.Password != user.Password || current.Role != user.Role || current.Status != user.Status {
			user.TokenVersion++
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := assignUserNumber(tx, &user); err != nil {
				return err
			}
			return tx.Create(&user).Error
		}
		user.PublicID = current.PublicID
		return tx.Save(&user).Error
	})
	return user, err
}

func RevokeUserSessions(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", id).UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
}

func UpdateUserLogin(id, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{"last_login_at": timestamp, "updated_at": timestamp}).Error
}

func UpdateUserProfile(id, displayName, avatarURL, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{"display_name": displayName, "avatar_url": avatarURL, "updated_at": timestamp}).Error
}

func UpdateUserPassword(id, previousHash, passwordHash, timestamp string) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	result := db.Model(&model.User{}).Where("id = ? AND password = ?", id, previousHash).Updates(map[string]any{"password": passwordHash, "token_version": gorm.Expr("token_version + 1"), "updated_at": timestamp})
	return result.RowsAffected == 1, result.Error
}

// GetUserByEmail queries a normalized email and also covers users created before email_key existed.
func GetUserByEmail(email string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "email_key = ? OR LOWER(email) = ?", email, email)
}

// GetUserByAffCode returns the active owner of an invitation code.
func GetUserByAffCode(affCode string) (model.User, bool, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, false, err
	}
	return findUser(db, "aff_code = ? AND status = ?", affCode, model.UserStatusActive)
}

// CreateUserWithCreditLog 原子创建注册用户及其初始额度流水。
func CreateUserWithCreditLog(user model.User, log *model.CreditLog) (model.User, error) {
	db, err := DB()
	if err != nil {
		return user, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := assignUserNumber(tx, &user); err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if log == nil {
			return nil
		}
		log.Balance = user.Credits
		return tx.Create(log).Error
	})
	return user, err
}

// SaveCreditLog 保存算力点变更流水。
func SaveCreditLog(log model.CreditLog) (model.CreditLog, error) {
	db, err := DB()
	if err != nil {
		return log, err
	}
	if n := searchUserNumber(log.UserID); n >= 0 {
		var user model.User
		if err := db.Select("id").Where("public_id = ?", n).Take(&user).Error; err != nil {
			return log, err
		}
		log.UserID = user.ID
	}
	return log, db.Save(&log).Error
}

func ListCreditLogs(q model.Query) ([]model.CreditLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	return listCreditLogs(db.Model(&model.CreditLog{}).Joins("LEFT JOIN users ON users.id = credit_logs.user_id"), q, true)
}

func ListUserCreditLogs(userID string, q model.Query) ([]model.CreditLog, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	return listCreditLogs(db.Model(&model.CreditLog{}).Where("user_id = ?", userID), q, false)
}

func listCreditLogs(tx *gorm.DB, q model.Query, includeUsername bool) ([]model.CreditLog, int64, error) {
	q.Normalize()
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		if includeUsername {
			tx = tx.Where("(users.public_id = ? OR credit_logs.user_id LIKE ? OR users.username LIKE ? OR credit_logs.type LIKE ? OR credit_logs.remark LIKE ? OR credit_logs.related_id LIKE ?)", searchUserNumber(keyword), like, like, like, like, like)
		} else {
			tx = tx.Where("(credit_logs.user_id LIKE ? OR credit_logs.type LIKE ? OR credit_logs.remark LIKE ? OR credit_logs.related_id LIKE ?)", like, like, like, like)
		}
	}
	if logType := strings.TrimSpace(q.Type); logType != "" {
		tx = tx.Where("credit_logs.type = ?", logType)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []model.CreditLog
	if includeUsername {
		tx = tx.Select("credit_logs.*, users.username AS username, users.public_id AS public_user_id")
	}
	err := tx.Order("credit_logs.created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&logs).Error
	return logs, total, err
}

func DeleteCreditLog(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.CreditLog{}, "id = ?", id).Error
}

// DeleteUser 删除指定用户。
func DeleteUser(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserAPIKey{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.UserPricingDiscount{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, "id = ?", id).Error
	})
}

// findUser 查询单个用户，并将未命中转换为 ok=false。
func findUser(db *gorm.DB, query string, args ...any) (model.User, bool, error) {
	user := model.User{}
	err := db.Where(query, args...).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, false, nil
	}
	return user, err == nil, err
}
