package middleware

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// abortIfEmailVerificationRequired stops a request from a user whose
// administrator requires a verified email they have not verified yet. Relay
// paths answer in the OpenAI error format; console paths in the API format.
func abortIfEmailVerificationRequired(c *gin.Context, userID int, openAIFormat bool) bool {
	blocked, err := model.EmailVerificationBlocks(userID)
	if err != nil {
		common.SysLog(common.LogText("email verification check failed for user %d: %v", userID, err))
		if openAIFormat {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, common.TranslateMessage(c, i18n.MsgDatabaseError))
		} else {
			common.ApiErrorStatus(c, http.StatusInternalServerError, errors.New(common.TranslateMessage(c, i18n.MsgDatabaseError)))
			c.Abort()
		}
		return true
	}
	if !blocked {
		return false
	}
	message := i18n.T(c, i18n.MsgAuthEmailVerificationRequired)
	if openAIFormat {
		abortWithOpenAiMessage(c, http.StatusForbidden, message, types.ErrorCodeAccessDenied)
	} else {
		common.ApiErrorStatus(c, http.StatusForbidden, errors.New(message))
		c.Abort()
	}
	return true
}

// VerifiedEmailRequired guards session-authenticated relay routes such as the
// playground; it must run after UserAuth.
func VerifiedEmailRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if abortIfEmailVerificationRequired(c, c.GetInt("id"), true) {
			return
		}
		c.Next()
	}
}
