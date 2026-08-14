package repository

import (
	"errors"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPaymentOrderNotFound = errors.New("payment order not found")
	ErrPaymentUserNotFound  = errors.New("payment user not found")
	ErrInsufficientBalance  = errors.New("insufficient balance")
)

func CreatePaymentOrder(order model.PaymentOrder) (model.PaymentOrder, error) {
	db, err := DB()
	if err != nil {
		return order, err
	}
	return order, db.Create(&order).Error
}

func GetPaymentOrderByOrderNo(orderNo string) (model.PaymentOrder, error) {
	db, err := DB()
	if err != nil {
		return model.PaymentOrder{}, err
	}
	var order model.PaymentOrder
	if err := db.Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.PaymentOrder{}, ErrPaymentOrderNotFound
		}
		return model.PaymentOrder{}, err
	}
	return order, nil
}

func GetUserPaymentOrder(userID string, orderNo string) (model.PaymentOrder, error) {
	db, err := DB()
	if err != nil {
		return model.PaymentOrder{}, err
	}
	var order model.PaymentOrder
	if err := db.Where("user_id = ? AND order_no = ?", userID, orderNo).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.PaymentOrder{}, ErrPaymentOrderNotFound
		}
		return model.PaymentOrder{}, err
	}
	return order, nil
}

func ListUserPaymentOrders(userID string, q model.Query) ([]model.PaymentOrder, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.PaymentOrder{}).Where("user_id = ?", userID)
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("order_no LIKE ? OR package_name LIKE ? OR trade_no LIKE ?", like, like, like)
	}
	if status := strings.TrimSpace(q.Type); status != "" {
		tx = tx.Where("status = ?", status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var orders []model.PaymentOrder
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&orders).Error
	return orders, total, err
}

func ListPendingPaymentOrders(createdAfter string, limit int) ([]model.PaymentOrder, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var orders []model.PaymentOrder
	err = db.Where("status = ? AND created_at >= ?", model.PaymentOrderStatusPending, createdAfter).
		Order("created_at desc").Limit(limit).Find(&orders).Error
	return orders, err
}

func SettlePaymentOrder(orderNo string, tradeNo string, paidAt string, log model.BalanceLog) (model.PaymentOrder, bool, error) {
	db, err := DB()
	if err != nil {
		return model.PaymentOrder{}, false, err
	}
	var order model.PaymentOrder
	settled := false
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", orderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPaymentOrderNotFound
			}
			return err
		}
		if order.Status == model.PaymentOrderStatusPaid {
			return nil
		}
		updated := tx.Model(&model.PaymentOrder{}).
			Where("id = ? AND status = ?", order.ID, model.PaymentOrderStatusPending).
			Updates(map[string]any{"status": model.PaymentOrderStatusPaid, "trade_no": tradeNo, "paid_at": paidAt, "updated_at": paidAt})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return nil
		}
		userUpdate := tx.Model(&model.User{}).Where("id = ?", order.UserID).Updates(map[string]any{
			"balance_cents": gorm.Expr("balance_cents + ?", order.BalanceCents), "updated_at": paidAt,
		})
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		if userUpdate.RowsAffected == 0 {
			return errors.New("payment user not found")
		}
		var user model.User
		if err := tx.Where("id = ?", order.UserID).First(&user).Error; err != nil {
			return err
		}
		log.UserID = order.UserID
		log.OrganizationID = order.OrganizationID
		log.RelatedID = order.ID
		log.AmountCents = order.BalanceCents
		log.BalanceCents = user.BalanceCents
		log.CreatedAt = paidAt
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		order.Status = model.PaymentOrderStatusPaid
		order.TradeNo = &tradeNo
		order.PaidAt = paidAt
		order.UpdatedAt = paidAt
		settled = true
		return nil
	})
	return order, settled, err
}

func ExchangeUserBalanceForCredits(userID string, balanceCents int64, credits int, updatedAt string, balanceLog model.BalanceLog, creditLog model.CreditLog) (model.User, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, err
	}
	var user model.User
	err = db.Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&model.User{}).
			Where("id = ? AND balance_cents >= ?", userID, balanceCents).
			Updates(map[string]any{
				"balance_cents": gorm.Expr("balance_cents - ?", balanceCents),
				"credits":       gorm.Expr("credits + ?", credits),
				"updated_at":    updatedAt,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			var count int64
			if err := tx.Model(&model.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrPaymentUserNotFound
			}
			return ErrInsufficientBalance
		}
		if err := tx.Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		balanceLog.UserID = userID
		balanceLog.AmountCents = -balanceCents
		balanceLog.BalanceCents = user.BalanceCents
		balanceLog.CreatedAt = updatedAt
		if err := tx.Create(&balanceLog).Error; err != nil {
			return err
		}
		creditLog.UserID = userID
		creditLog.CreditSource = model.CreditSourcePersonal
		creditLog.Amount = credits
		creditLog.Balance = user.Credits
		creditLog.CreatedAt = updatedAt
		return tx.Create(&creditLog).Error
	})
	return user, err
}
