package middleware

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// API clients get English whatever the console language is.
const userConcurrencyLimitMessage = "Concurrency limit reached. Please reduce concurrency and retry later."

// UserConcurrencyLimit caps the relay requests a user runs at once at the
// limit an administrator set for that user. The request holds its slot until
// the handler returns, so a stream or a realtime session keeps it until it
// ends. Mount it after TokenAuth on routes that start upstream work.
//
// Failing to read the policy or the shared count lets the request through:
// the limit is optional and must not make its storage a failure point for
// every relay request.
func UserConcurrencyLimit() gin.HandlerFunc {
	return userConcurrencyLimit(false)
}

// MidjourneyUserConcurrencyLimit is UserConcurrencyLimit answering in the
// Midjourney error format.
func MidjourneyUserConcurrencyLimit() gin.HandlerFunc {
	return userConcurrencyLimit(true)
}

func userConcurrencyLimit(midjourney bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt("id")
		if userID <= 0 {
			c.Next()
			return
		}
		snapshot, err := model.GetUserLimitSnapshot(userID)
		if err != nil {
			common.SysError(common.LogText("failed to load the usage limits of user %d: %s", userID, err.Error()))
			c.Next()
			return
		}
		if snapshot.MaxConcurrency <= 0 {
			c.Next()
			return
		}
		lease, acquired, err := service.AcquireUserConcurrency(userID, snapshot.MaxConcurrency)
		if err != nil {
			common.SysError(common.LogText("failed to count the running requests of user %d: %s", userID, err.Error()))
			c.Next()
			return
		}
		if !acquired {
			c.Header("Retry-After", strconv.Itoa(service.UserConcurrencyRetryAfter))
			if midjourney {
				abortWithMidjourneyMessage(c, http.StatusTooManyRequests, constant.MjUserLimitReached, userConcurrencyLimitMessage)
				return
			}
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, userConcurrencyLimitMessage, types.ErrorCodeUserConcurrencyLimitReached)
			return
		}
		defer lease.Release()
		c.Next()
	}
}
