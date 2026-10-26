package model

import (
	"fmt"
	"strconv"
)

const AdobeFreeTrialSuccessLimit = 3

// Adobe paid-points accounts use the existing 10,000-credit classification.
func (a TokenAccount) AdobeFreeAccount() bool {
	total, _ := strconv.ParseFloat(fmt.Sprint(a.Meta["cached_quota_total"]), 64)
	return a.Pool == "adobe" && total < 10000
}

func (a TokenAccount) AdobeTrialExhausted() bool {
	return a.AdobeFreeAccount() && a.SuccessTotal >= AdobeFreeTrialSuccessLimit
}
