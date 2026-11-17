package service

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ErrEmailVerificationStepUpAvailable means the account can verify its
// identity, so a first address must go through the regular email binding.
var ErrEmailVerificationStepUpAvailable = errors.New("Verify your identity before binding an email address.")

type EmailVerificationData struct {
	FlowToken string `json:"flow_token"`
	Email     string `json:"email"`
	Mode      string `json:"mode"`
	ExpiresAt int64  `json:"expires_at"`
	ResendAt  int64  `json:"resend_at"`
}

// StartEmailVerification sends a code for a user whose administrator requires a
// verified email. Without an address it verifies the current one; with one it
// binds a first address, which is only allowed when the account has no way to
// verify its identity (otherwise the regular binding with step-up applies).
func StartEmailVerification(identity AuthIdentity, email string, lang string) (*EmailVerificationData, error) {
	user, err := model.GetUserById(identity.UserID, false)
	if err != nil {
		return nil, err
	}
	if !user.RequireVerifiedEmail {
		return nil, model.ErrEmailVerificationNotRequired
	}
	current := model.NormalizeEmail(user.Email)
	state := model.EmailVerificationState{ResendAt: time.Now().Add(model.EmailBindingResendDelay).Unix()}
	if email == "" {
		if current == "" {
			return nil, model.ErrEmailVerificationNoEmail
		}
		if user.EmailVerified {
			return nil, model.ErrEmailVerificationNotRequired
		}
		state.Mode, state.Email = model.EmailVerificationModeCurrent, current
	} else {
		if current != "" {
			return nil, model.ErrEmailVerificationChanged
		}
		hasStepUp, err := hasAvailableStepUpMethod(identity)
		if err != nil {
			return nil, err
		}
		if hasStepUp {
			return nil, ErrEmailVerificationStepUpAvailable
		}
		normalized, err := ValidateAccountEmail(email)
		if err != nil {
			return nil, err
		}
		state.Mode, state.Email = model.EmailVerificationModeRequired, normalized
	}

	codes, err := generateEmailBindingCodes(false)
	if err != nil {
		return nil, err
	}
	state.NewCodeHash = codes.NewHash
	token, flow, err := model.CreateEmailVerification(identity, state)
	if err != nil {
		return nil, err
	}
	if err := sendEmailBindingCodes(lang, model.EmailBindingState{Email: state.Email}, codes); err != nil {
		// Never leave a usable flow whose code was not delivered.
		_, _ = model.ConsumeAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeEmailVerification, UserId: identity.UserID, SessionId: identity.SessionID})
		return nil, ErrEmailBindingDelivery
	}
	return emailVerificationData(token, flow, &state), nil
}

func ResendEmailVerification(identity AuthIdentity, token string, lang string) (*EmailVerificationData, error) {
	_, state, err := model.GetEmailVerification(identity, token)
	if err != nil {
		return nil, err
	}
	if state.ResendAt > time.Now().Unix() {
		return nil, model.ErrEmailBindingResendWait
	}
	if state.FailedAttempts >= model.EmailBindingMaxAttempts {
		return nil, model.ErrEmailBindingLocked
	}
	codes, err := generateEmailBindingCodes(false)
	if err != nil {
		return nil, err
	}
	flow, state, err := model.ResendEmailVerification(identity, token, codes.NewHash)
	if err != nil {
		return nil, err
	}
	if err := sendEmailBindingCodes(lang, model.EmailBindingState{Email: state.Email}, codes); err != nil {
		_, _ = model.ConsumeAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeEmailVerification, UserId: identity.UserID, SessionId: identity.SessionID})
		return nil, ErrEmailBindingDelivery
	}
	return emailVerificationData(token, flow, state), nil
}

func FinishEmailVerification(identity AuthIdentity, token, code string) (*model.EmailVerificationState, error) {
	_, state, err := model.GetEmailVerification(identity, token)
	if err != nil {
		return nil, err
	}
	if state.Mode == model.EmailVerificationModeRequired {
		// The policy may have changed since the code was sent.
		if hasStepUp, err := hasAvailableStepUpMethod(identity); err != nil {
			return nil, err
		} else if hasStepUp {
			return nil, ErrEmailVerificationStepUpAvailable
		}
		if _, err := ValidateAccountEmail(state.Email); err != nil {
			return nil, err
		}
	}
	return model.CompleteEmailVerification(identity, token, state.Email, code)
}

func hasAvailableStepUpMethod(identity AuthIdentity) (bool, error) {
	requirements, err := GetVerificationRequirements(identity, VerificationScopeAccountBind)
	if err != nil {
		return false, err
	}
	for _, method := range requirements.Methods {
		if method.Available {
			return true, nil
		}
	}
	return false, nil
}

func emailVerificationData(token string, flow *model.AuthFlow, state *model.EmailVerificationState) *EmailVerificationData {
	return &EmailVerificationData{
		FlowToken: token, Email: state.Email, Mode: state.Mode,
		ExpiresAt: flow.ExpiresAt.Unix(), ResendAt: state.ResendAt,
	}
}
