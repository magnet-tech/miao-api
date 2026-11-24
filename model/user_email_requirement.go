package model

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// An administrator can require a user to verify an email address before the
// user's API keys work. These helpers decide whether a user is blocked and run
// the code flow that verifies the current address or binds a first one.

const (
	AuthFlowPurposeEmailVerification = "email_verification"

	EmailVerificationModeCurrent  = "current"
	EmailVerificationModeRequired = "required_bind"

	emailGateTTL = 60 * time.Second
)

// These reach the web console as message keys, so it translates them.
var (
	ErrEmailVerificationNotRequired error = common.NewMessage("This account does not need email verification.")
	ErrEmailVerificationNoEmail     error = common.NewMessage("This account has no email address to verify.")
	ErrEmailVerificationChanged     error = common.NewMessage("The account email changed. Start email verification again.")
)

// EmailVerificationState is the server-owned payload of an email verification
// flow: the address, a salted code hash and the attempt counter.
type EmailVerificationState struct {
	Mode           string `json:"mode"`
	Email          string `json:"email"`
	NewCodeHash    string `json:"new_code_hash"`
	FailedAttempts int    `json:"failed_attempts"`
	ResendAt       int64  `json:"resend_at"`
}

type emailGateEntry struct {
	blocked bool
	expires time.Time
}

var (
	emailGateMu    sync.Mutex
	emailGateLocal = map[int]emailGateEntry{}
)

func emailGateKey(userID int) string {
	return fmt.Sprintf("user_email_gate:%d", userID)
}

// EmailVerificationBlocks reports whether the user's API access is paused until
// an email address is verified. Results are cached for a minute; every change
// to the requirement, the address or its verification clears the cache.
func EmailVerificationBlocks(userID int) (bool, error) {
	if common.RedisEnabled {
		if value, err := common.RedisGet(emailGateKey(userID)); err == nil {
			return value == "1", nil
		}
	} else {
		emailGateMu.Lock()
		entry, ok := emailGateLocal[userID]
		emailGateMu.Unlock()
		if ok && time.Now().Before(entry.expires) {
			return entry.blocked, nil
		}
	}

	var user User
	err := DB.Select("id", "role", "email", "email_verified", "require_verified_email").First(&user, userID).Error
	if err != nil {
		return false, err
	}
	blocked := user.Role < common.RoleRootUser && user.RequireVerifiedEmail &&
		(NormalizeEmail(user.Email) == "" || !user.EmailVerified)

	if common.RedisEnabled {
		value := "0"
		if blocked {
			value = "1"
		}
		if err := common.RedisSet(emailGateKey(userID), value, emailGateTTL); err != nil {
			common.SysLog(common.LogText("failed to cache email verification gate for user %d: %s", userID, err.Error()))
		}
	} else {
		emailGateMu.Lock()
		emailGateLocal[userID] = emailGateEntry{blocked: blocked, expires: time.Now().Add(emailGateTTL)}
		emailGateMu.Unlock()
	}
	return blocked, nil
}

// InvalidateEmailVerificationGate drops the cached gate decision after the
// requirement, the address or its verification changes.
func InvalidateEmailVerificationGate(userID int) {
	if common.RedisEnabled {
		if err := common.RedisDel(emailGateKey(userID)); err != nil {
			common.SysLog(common.LogText("failed to clear email verification gate for user %d: %s", userID, err.Error()))
		}
		return
	}
	emailGateMu.Lock()
	delete(emailGateLocal, userID)
	emailGateMu.Unlock()
}

// SetUserRequireVerifiedEmailWithTx changes the requirement only; callers clear
// the gate cache after the transaction commits.
func SetUserRequireVerifiedEmailWithTx(tx *gorm.DB, userID int, require bool) error {
	return tx.Model(&User{}).Where("id = ?", userID).Update("require_verified_email", require).Error
}

// CreateEmailVerification starts a flow for the current address, or for a first
// address when the account qualifies for a binding without step-up.
func CreateEmailVerification(identity AuthSessionIdentity, state EmailVerificationState) (string, *AuthFlow, error) {
	var token string
	var flow *AuthFlow
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := validateEmailVerificationWithTx(tx, identity, &state); err != nil {
			return err
		}
		if state.Mode == EmailVerificationModeRequired {
			if err := ensureEmailAvailableWithTx(tx, state.Email, identity.UserID); err != nil {
				return err
			}
		}
		payload, err := common.Marshal(state)
		if err != nil {
			return err
		}
		token, flow, err = createAuthFlowWithTx(tx, AuthFlowCreate{
			Purpose: AuthFlowPurposeEmailVerification, UserId: identity.UserID, SessionId: identity.SessionID,
			Payload: string(payload), ExpiresAt: time.Now().Add(EmailBindingTTL),
		})
		return err
	})
	return token, flow, err
}

func GetEmailVerification(identity AuthSessionIdentity, token string) (*AuthFlow, *EmailVerificationState, error) {
	flow, err := GetAuthFlow(token, AuthFlowMatch{Purpose: AuthFlowPurposeEmailVerification, UserId: identity.UserID, SessionId: identity.SessionID})
	if err != nil {
		return nil, nil, err
	}
	var state EmailVerificationState
	if err := common.UnmarshalJsonStr(flow.Payload, &state); err != nil {
		return nil, nil, ErrAuthFlowInvalid
	}
	return flow, &state, nil
}

// ResendEmailVerification keeps the deadline and the attempt counter.
func ResendEmailVerification(identity AuthSessionIdentity, token, newCodeHash string) (*AuthFlow, *EmailVerificationState, error) {
	var flow *AuthFlow
	var state *EmailVerificationState
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		flow, state, err = lockEmailVerificationWithTx(tx, identity, token)
		if err != nil {
			return err
		}
		now := time.Now()
		if state.ResendAt > now.Unix() {
			return ErrEmailBindingResendWait
		}
		if newCodeHash == "" {
			return ErrAuthFlowInvalid
		}
		state.NewCodeHash = newCodeHash
		state.ResendAt = now.Add(EmailBindingResendDelay).Unix()
		payload, err := common.Marshal(state)
		if err != nil {
			return err
		}
		return tx.Model(flow).Update("payload", string(payload)).Error
	})
	return flow, state, err
}

// CompleteEmailVerification checks the code and marks the address verified,
// writing a first address in the same transaction for a required binding.
// Failed attempts are committed even though verification fails.
func CompleteEmailVerification(identity AuthSessionIdentity, token, email, code string) (*EmailVerificationState, error) {
	var state *EmailVerificationState
	var verificationError error
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Take the email ownership lock before the first consistent read, as
		// CompleteEmailBinding does, so a MySQL REPEATABLE READ snapshot cannot
		// hide an address claimed concurrently by another account.
		if email == "" || email != NormalizeEmail(email) {
			return ErrAuthFlowInvalid
		}
		if err := lockNormalizedEmail(tx, email); err != nil {
			return err
		}
		flow, lockedState, err := lockEmailVerificationWithTx(tx, identity, token)
		if err != nil {
			return err
		}
		state = lockedState
		if state.Email != email {
			return ErrAuthFlowInvalid
		}
		code, codeError := common.ValidateNumericCode(code)
		if codeError != nil || !common.ValidatePasswordAndHash(code, state.NewCodeHash) {
			state.FailedAttempts++
			verificationError = ErrEmailBindingCodeInvalid
			if state.FailedAttempts >= EmailBindingMaxAttempts {
				verificationError = ErrEmailBindingLocked
			}
			payload, err := common.Marshal(state)
			if err != nil {
				return err
			}
			return tx.Model(flow).Update("payload", string(payload)).Error
		}
		updates := map[string]any{"email_verified": true}
		if state.Mode == EmailVerificationModeRequired {
			if err := ensureEmailAvailableWithTx(tx, state.Email, identity.UserID); err != nil {
				return err
			}
			updates["email"] = state.Email
		}
		if err := tx.Model(&User{}).Where("id = ?", identity.UserID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(flow).Update("consumed_at", time.Now()).Error
	})
	if err != nil {
		return nil, err
	}
	if verificationError == nil {
		InvalidateEmailVerificationGate(identity.UserID)
	}
	return state, verificationError
}

func lockEmailVerificationWithTx(tx *gorm.DB, identity AuthSessionIdentity, token string) (*AuthFlow, *EmailVerificationState, error) {
	match := AuthFlowMatch{Purpose: AuthFlowPurposeEmailVerification, UserId: identity.UserID, SessionId: identity.SessionID}
	// A no-op write first, as in lockEmailBindingWithTx: SQLite avoids a
	// deferred-transaction upgrade and MySQL/PostgreSQL lock the row.
	if err := applyAuthFlowMatch(tx.Model(&AuthFlow{}), token, match).
		Where("consumed_at IS NULL AND expires_at > ?", time.Now()).
		UpdateColumn("payload", gorm.Expr("payload")).Error; err != nil {
		return nil, nil, err
	}
	var flow AuthFlow
	if err := applyAuthFlowMatch(tx, token, match).First(&flow).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrAuthFlowInvalid
		}
		return nil, nil, err
	}
	if flow.ConsumedAt != nil {
		return nil, nil, ErrAuthFlowConsumed
	}
	if !flow.ExpiresAt.After(time.Now()) {
		return nil, nil, ErrAuthFlowExpired
	}
	var state EmailVerificationState
	if err := common.UnmarshalJsonStr(flow.Payload, &state); err != nil {
		return nil, nil, ErrAuthFlowInvalid
	}
	if err := validateEmailVerificationWithTx(tx, identity, &state); err != nil {
		return nil, nil, err
	}
	if state.FailedAttempts >= EmailBindingMaxAttempts {
		return nil, nil, ErrEmailBindingLocked
	}
	return &flow, &state, nil
}

// validateEmailVerificationWithTx re-checks, under the flow's transaction, that
// the account still matches the flow: the same current address, or still no
// address while the administrator's requirement is on.
func validateEmailVerificationWithTx(tx *gorm.DB, identity AuthSessionIdentity, state *EmailVerificationState) error {
	if state.Email == "" || state.Email != NormalizeEmail(state.Email) || state.NewCodeHash == "" {
		return ErrAuthFlowInvalid
	}
	if err := ValidateAuthSessionWithTx(tx, identity); err != nil {
		return err
	}
	var user User
	if err := tx.Select("id", "email", "require_verified_email").First(&user, identity.UserID).Error; err != nil {
		return err
	}
	current := NormalizeEmail(user.Email)
	switch state.Mode {
	case EmailVerificationModeCurrent:
		if current != state.Email {
			return ErrEmailVerificationChanged
		}
	case EmailVerificationModeRequired:
		if current != "" || !user.RequireVerifiedEmail {
			return ErrEmailVerificationChanged
		}
	default:
		return ErrAuthFlowInvalid
	}
	return nil
}
