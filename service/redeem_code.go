package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const redemptionAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

func ListRedemptionCodes(q model.Query) (model.RedemptionCodeList, error) {
	codes, total, err := repository.ListRedemptionCodes(q)
	if err != nil {
		return model.RedemptionCodeList{}, err
	}
	return model.RedemptionCodeList{Items: codes, Total: int(total)}, nil
}

func GenerateRedemptionCodes(credits int, quantity int, prefix string, remark string) ([]model.RedemptionCode, error) {
	if credits <= 0 {
		return nil, safeMessageError{message: "兑换点数必须大于 0"}
	}
	if quantity <= 0 {
		return nil, safeMessageError{message: "生成数量必须大于 0"}
	}
	if quantity > 500 {
		return nil, safeMessageError{message: "单次最多生成 500 个兑换码"}
	}

	generatedAt := now()
	prefix = normalizeRedemptionCodePrefix(prefix)
	remark = strings.TrimSpace(remark)
	codes := make([]model.RedemptionCode, 0, quantity)
	seen := map[string]bool{}
	for len(codes) < quantity {
		codeText, err := randomRedemptionCode(prefix)
		if err != nil {
			return nil, err
		}
		if seen[codeText] {
			continue
		}
		seen[codeText] = true
		codes = append(codes, model.RedemptionCode{
			ID:        newID("redeem"),
			Code:      codeText,
			Credits:   credits,
			Status:    model.RedemptionCodeStatusActive,
			Remark:    remark,
			CreatedAt: generatedAt,
			UpdatedAt: generatedAt,
		})
	}
	return repository.CreateRedemptionCodes(codes)
}

func DeleteRedemptionCode(id string) error {
	if strings.TrimSpace(id) == "" {
		return safeMessageError{message: "兑换码不存在"}
	}
	return repository.DeleteRedemptionCode(id)
}

func DeleteRedemptionCodes(ids []string, status string) (int64, error) {
	cleanIDs := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleanIDs = append(cleanIDs, id)
	}
	status = strings.TrimSpace(status)
	if len(cleanIDs) == 0 && status == "" {
		return 0, safeMessageError{message: "请选择要删除的兑换码"}
	}
	if status != "" && status != string(model.RedemptionCodeStatusUsed) && status != string(model.RedemptionCodeStatusActive) && status != string(model.RedemptionCodeStatusDisabled) {
		return 0, safeMessageError{message: "兑换码状态不支持"}
	}
	return repository.DeleteRedemptionCodes(cleanIDs, status)
}

func RedeemCode(userID string, code string) (model.AuthUser, error) {
	code = normalizeRedemptionCode(code)
	if strings.TrimSpace(userID) == "" {
		return model.AuthUser{}, safeMessageError{message: "请先登录"}
	}
	if code == "" {
		return model.AuthUser{}, safeMessageError{message: "请输入兑换码"}
	}

	redeemedAt := now()
	_, user, err := repository.RedeemRedemptionCode(userID, code, redeemedAt, model.CreditLog{
		ID:     newID("credit"),
		Type:   model.CreditLogTypeRedeem,
		Remark: "兑换码充值",
	})
	if err != nil {
		if errors.Is(err, repository.ErrRedemptionCodeNotFound) {
			return model.AuthUser{}, safeMessageError{message: "兑换码不存在"}
		}
		if errors.Is(err, repository.ErrRedemptionCodeUnavailable) {
			return model.AuthUser{}, safeMessageError{message: "兑换码已使用或不可用"}
		}
		if errors.Is(err, repository.ErrRedemptionUserNotFound) {
			return model.AuthUser{}, safeMessageError{message: "用户不存在"}
		}
		return model.AuthUser{}, err
	}
	return model.PublicUser(user), nil
}

func randomRedemptionCode(prefix string) (string, error) {
	chunks := make([]string, 3)
	for i := range chunks {
		chunk, err := randomRedemptionChunk(4)
		if err != nil {
			return "", err
		}
		chunks[i] = chunk
	}
	code := strings.Join(chunks, "-")
	if prefix != "" {
		return prefix + "-" + code, nil
	}
	return code, nil
}

func randomRedemptionChunk(size int) (string, error) {
	var builder strings.Builder
	builder.Grow(size)
	max := big.NewInt(int64(len(redemptionAlphabet)))
	for builder.Len() < size {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		builder.WriteByte(redemptionAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func normalizeRedemptionCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToUpper(value)
}

func normalizeRedemptionCodePrefix(value string) string {
	value = normalizeRedemptionCode(value)
	value = strings.ReplaceAll(value, "-", "")
	if len(value) > 12 {
		value = value[:12]
	}
	return value
}
