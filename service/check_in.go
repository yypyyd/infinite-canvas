package service

import (
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

var checkInLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func CheckInStatus(userID string) (model.CheckInStatus, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.CheckInStatus{}, err
	}
	setting := normalizeSettings(settings).Public.CheckIn
	date := checkInDate()
	checked, err := repository.HasCheckIn(userID, date)
	if err != nil {
		return model.CheckInStatus{}, err
	}
	return model.CheckInStatus{
		Enabled:        setting.Enabled,
		Reward:         setting.Reward,
		RewardCredits:  setting.RewardCredits,
		CheckedInToday: checked,
		CheckInDate:    date,
	}, nil
}

func CheckIn(userID string) (model.CheckInResult, error) {
	status, err := CheckInStatus(userID)
	if err != nil {
		return model.CheckInResult{}, err
	}
	if !status.Enabled {
		return model.CheckInResult{}, safeMessageError{message: "签到功能未开启"}
	}
	if status.CheckedInToday {
		return model.CheckInResult{}, safeMessageError{message: "今日已签到"}
	}
	reward := 0
	if status.Reward {
		reward = status.RewardCredits
	}
	createdAt := now()
	checkIn := model.CheckIn{ID: newID("checkin"), UserID: userID, CheckInDate: status.CheckInDate, RewardCredits: reward, CreatedAt: createdAt}
	var log *model.CreditLog
	if reward > 0 {
		log = &model.CreditLog{
			ID:        newID("credit"),
			UserID:    userID,
			Type:      model.CreditLogTypeCheckIn,
			Amount:    reward,
			RelatedID: checkIn.ID,
			Remark:    "每日签到奖励",
			CreatedAt: createdAt,
		}
	}
	user, err := repository.CreateCheckIn(checkIn, log, createdAt)
	if err != nil {
		return model.CheckInResult{}, safeMessageError{message: "今日已签到或签到失败，请稍后重试"}
	}
	return model.CheckInResult{RewardCredits: reward, CheckInDate: status.CheckInDate, User: model.PublicUser(user)}, nil
}

func checkInDate() string {
	return time.Now().In(checkInLocation).Format("2006-01-02")
}
