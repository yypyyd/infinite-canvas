package model

type PaymentMethod string

const (
	PaymentMethodAlipay PaymentMethod = "alipay"
	PaymentMethodWxpay  PaymentMethod = "wxpay"
	PaymentMethodQQPay  PaymentMethod = "qqpay"
)

type PaymentOrderStatus string

const (
	PaymentOrderStatusPending PaymentOrderStatus = "pending"
	PaymentOrderStatusPaid    PaymentOrderStatus = "paid"
)

type PaymentPackage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AmountCents int64  `json:"amountCents"`
}

type PaymentSetting struct {
	Enabled               bool             `json:"enabled"`
	GatewayURL            string           `json:"gatewayUrl"`
	MerchantID            string           `json:"merchantId"`
	MerchantKey           string           `json:"merchantKey"`
	MerchantKeyConfigured bool             `json:"merchantKeyConfigured"`
	SiteName              string           `json:"siteName"`
	ProductName           string           `json:"productName"`
	Methods               []PaymentMethod  `json:"methods"`
	Packages              []PaymentPackage `json:"packages"`
	CreditsPerYuan        int              `json:"creditsPerYuan"`
}

type PaymentConfig struct {
	Enabled  bool             `json:"enabled"`
	Methods  []PaymentMethod  `json:"methods"`
	Packages []PaymentPackage `json:"packages"`
	CreditsPerYuan int         `json:"creditsPerYuan"`
}

type PaymentOrder struct {
	ID             string             `json:"id" gorm:"primaryKey"`
	OrderNo        string             `json:"orderNo" gorm:"uniqueIndex"`
	UserID         string             `json:"userId" gorm:"index"`
	OrganizationID string             `json:"organizationId" gorm:"index"`
	PackageID      string             `json:"packageId"`
	PackageName    string             `json:"packageName"`
	Method         PaymentMethod      `json:"method"`
	AmountCents    int64              `json:"amountCents"`
	Status         PaymentOrderStatus `json:"status" gorm:"index"`
	TradeNo        *string            `json:"tradeNo,omitempty" gorm:"uniqueIndex"`
	PaidAt         string             `json:"paidAt"`
	CreatedAt      string             `json:"createdAt"`
	UpdatedAt      string             `json:"updatedAt"`
}

type BalanceLogType string

const (
	BalanceLogTypePaymentRecharge BalanceLogType = "payment_recharge"
	BalanceLogTypeCreditsExchange BalanceLogType = "credits_exchange"
)

type BalanceLog struct {
	ID             string         `json:"id" gorm:"primaryKey"`
	UserID         string         `json:"userId" gorm:"index"`
	OrganizationID string         `json:"organizationId" gorm:"index"`
	Type           BalanceLogType `json:"type" gorm:"index"`
	AmountCents    int64          `json:"amountCents"`
	BalanceCents   int64          `json:"balanceCents"`
	RelatedID      string         `json:"relatedId" gorm:"index"`
	Remark         string         `json:"remark"`
	Extra          string         `json:"extra" gorm:"type:text"`
	CreatedAt      string         `json:"createdAt" gorm:"index"`
}

type PaymentOrderList struct {
	Items []PaymentOrder `json:"items"`
	Total int            `json:"total"`
}

type BalanceExchangeResult struct {
	ExchangeID        string `json:"exchangeId"`
	BalanceCents      int64  `json:"balanceCents"`
	PersonalCredits   int    `json:"personalCredits"`
	SpentBalanceCents int64  `json:"spentBalanceCents"`
	ReceivedCredits   int    `json:"receivedCredits"`
}

type PaymentSubmission struct {
	Order     PaymentOrder      `json:"order"`
	SubmitURL string            `json:"submitUrl"`
	Params    map[string]string `json:"params"`
}
