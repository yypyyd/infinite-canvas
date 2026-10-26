package model

import (
	"fmt"
	"strconv"
)

// Adobe paid-points accounts use the existing 10,000-credit classification.
func (a TokenAccount) AdobeFreeAccount() bool {
	total, _ := strconv.ParseFloat(fmt.Sprint(a.Meta["cached_quota_total"]), 64)
	return a.Pool == "adobe" && total < 10000
}

func (a TokenAccount) AdobeTrialExhausted() bool {
	return a.Pool == "adobe" && a.ImageLimited && a.VideoLimited
}
