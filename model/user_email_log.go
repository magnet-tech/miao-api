package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Emails administrators send to users: what was sent and whether delivery
// succeeded. The body is not stored.

const (
	UserEmailKindCustom    = "custom"
	UserEmailKindSuspended = "suspended"
	UserEmailKindRestored  = "restored"

	AuthFlowPurposeAccountNotice = "account_notice"
	AccountNoticeTTL             = 10 * time.Minute

	userEmailHistoryLimit = 50
)

var ErrAccountNoticeUnavailable = errors.New("This notice can no longer be sent. Send it from the email drawer instead.")

type UserEmailLog struct {
	Id               int    `json:"id"`
	UserId           int    `json:"user_id" gorm:"index:idx_user_email_log_user_time,priority:1"`
	OperatorId       int    `json:"operator_id"`
	OperatorUsername string `json:"operator" gorm:"type:varchar(64)"`
	Kind             string `json:"kind" gorm:"type:varchar(16)"`
	Subject          string `json:"subject" gorm:"type:varchar(255)"`
	Success          bool   `json:"success"`
	CreatedAt        int64  `json:"time" gorm:"type:bigint;index:idx_user_email_log_user_time,priority:2"`
}

func RecordUserEmail(entry UserEmailLog) {
	entry.CreatedAt = common.GetTimestamp()
	if err := DB.Create(&entry).Error; err != nil {
		common.SysError(common.LogText("failed to record email to user %d: %s", entry.UserId, err.Error()))
	}
}

func GetUserEmailHistory(userID int) ([]UserEmailLog, error) {
	var entries []UserEmailLog
	err := DB.Where("user_id = ?", userID).Order("created_at desc, id desc").Limit(userEmailHistoryLimit).Find(&entries).Error
	return entries, err
}

type accountNoticeGrant struct {
	TargetUserID int    `json:"target_user_id"`
	Kind         string `json:"kind"`
}

// CreateAccountNoticeGrant is issued after a verified disable or enable: one
// notice of that kind to that user, from the same operator session, once.
func CreateAccountNoticeGrant(operatorID int, sessionID string, targetUserID int, kind string) (string, time.Time, error) {
	payload, err := common.Marshal(accountNoticeGrant{TargetUserID: targetUserID, Kind: kind})
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(AccountNoticeTTL)
	var token string
	err = DB.Transaction(func(tx *gorm.DB) error {
		var err error
		token, _, err = createAuthFlowWithTx(tx, AuthFlowCreate{
			Purpose: AuthFlowPurposeAccountNotice, UserId: operatorID, SessionId: sessionID,
			Payload: string(payload), ExpiresAt: expiresAt,
		})
		return err
	})
	return token, expiresAt, err
}

// ConsumeAccountNoticeGrant spends the grant whether or not it matches, so a
// guessed or reused token never sends anything.
func ConsumeAccountNoticeGrant(token string, operatorID int, sessionID string, targetUserID int, kind string) error {
	if token == "" {
		return ErrAccountNoticeUnavailable
	}
	flow, err := ConsumeAuthFlow(token, AuthFlowMatch{Purpose: AuthFlowPurposeAccountNotice, UserId: operatorID, SessionId: sessionID})
	if err != nil {
		return ErrAccountNoticeUnavailable
	}
	var grant accountNoticeGrant
	if err := common.UnmarshalJsonStr(flow.Payload, &grant); err != nil {
		return ErrAccountNoticeUnavailable
	}
	if grant.TargetUserID != targetUserID || grant.Kind != kind {
		return ErrAccountNoticeUnavailable
	}
	return nil
}
