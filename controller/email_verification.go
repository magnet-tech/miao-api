package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type emailVerificationRequest struct {
	Email     string `json:"email"`
	FlowToken string `json:"flow_token"`
	Code      string `json:"code"`
}

// writeEmailVerificationError maps the errors specific to the required email
// verification and falls back to the shared security error responses.
func writeEmailVerificationError(c *gin.Context, err error) {
	var code string
	switch {
	case errors.Is(err, model.ErrEmailVerificationNotRequired):
		code = "EMAIL_VERIFICATION_NOT_REQUIRED"
	case errors.Is(err, model.ErrEmailVerificationNoEmail):
		code = "EMAIL_VERIFICATION_NO_EMAIL"
	case errors.Is(err, model.ErrEmailVerificationChanged):
		code = "EMAIL_VERIFICATION_CHANGED"
	case errors.Is(err, service.ErrEmailVerificationStepUpAvailable):
		code = "EMAIL_VERIFICATION_STEP_UP_REQUIRED"
	default:
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiErrorStatus(c, http.StatusOK, err, gin.H{"code": code})
}

// EmailVerificationStart sends a code to the current address, or to a first
// address when the account has no way to verify its identity.
func EmailVerificationStart(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	var request emailVerificationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeSecurityOperationError(c, service.ErrVerificationContextInvalid)
		return
	}
	data, err := service.StartEmailVerification(identity, request.Email, service.AccountMailLanguage(c, identity.UserID))
	succeeded := err == nil
	params := map[string]any{"success": succeeded}
	if data != nil {
		params["mode"] = data.Mode
	}
	recordUserSecurityAudit(c, identity.UserID, "user.email_verification_start", params)
	if err != nil {
		writeEmailVerificationError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func EmailVerificationResend(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	var request emailVerificationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.FlowToken == "" {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return
	}
	data, err := service.ResendEmailVerification(identity, request.FlowToken, service.AccountMailLanguage(c, identity.UserID))
	if err != nil {
		writeEmailVerificationError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func EmailVerificationFinish(c *gin.Context) {
	identity, ok := middleware.GetStepUpIdentity(c)
	if !ok {
		writeSecurityOperationError(c, service.ErrAuthTokenInvalid)
		return
	}
	var request emailVerificationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.FlowToken == "" {
		writeSecurityOperationError(c, model.ErrAuthFlowInvalid)
		return
	}
	state, err := service.FinishEmailVerification(identity, request.FlowToken, request.Code)
	params := map[string]any{"success": err == nil}
	if state != nil {
		params["mode"] = state.Mode
	}
	recordUserSecurityAudit(c, identity.UserID, "user.email_verification_finish", params)
	if err != nil {
		writeEmailVerificationError(c, err)
		return
	}
	if err := model.PublishUserAuthCache(identity.UserID); err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"email": state.Email, "email_verified": true})
}
