package service

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// CheckUserUsageLimit rejects a request with 429 and Retry-After while an
// enabled usage rule that covers modelName is used up. Like a subscription
// limit it blocks only once a window is spent, so requests already running may
// settle slightly past the limit. pendingQuota is quota charged but not yet
// recorded in the windows (a running realtime session).
//
// Failing to read the policy or the windows lets the request through: the
// limit is optional and must not make its storage a failure point for every
// relay request, including those of users without limits.
func CheckUserUsageLimit(c *gin.Context, userID int, modelName string, pendingQuota int64) *types.NewAPIError {
	if userID <= 0 {
		return nil
	}
	snapshot, err := model.GetUserLimitSnapshot(userID)
	if err != nil {
		common.SysError(common.LogText("failed to load the usage limits of user %d: %s", userID, err.Error()))
		return nil
	}
	if !slices.ContainsFunc(snapshot.Rules, func(rule model.UserUsageRule) bool {
		return rule.Enabled && rule.AppliesTo(modelName)
	}) {
		return nil
	}
	windows, err := model.GetUserUsageWindows(userID)
	if err != nil {
		common.SysError(common.LogText("failed to load the usage windows of user %d: %s", userID, err.Error()))
		return nil
	}
	now := common.GetTimestamp()
	var blocking *model.UserUsageRule
	var resetAt int64
	for _, rule := range snapshot.Rules {
		if !rule.Enabled || !rule.AppliesTo(modelName) {
			continue
		}
		state := rule.State(windows[rule.WindowSeconds], now)
		if state.UsedQuota+pendingQuota < rule.QuotaLimit {
			continue
		}
		if !state.Active && !rule.IsCalendar() {
			// Only the pending charge counts: it opened the window now.
			state.ResetAt = now + rule.WindowSeconds
		}
		// With several windows spent the request passes only once the last
		// of them resets, so that one is reported.
		if blocking == nil || state.ResetAt > resetAt {
			blocking, resetAt = &rule, state.ResetAt
		}
	}
	if blocking == nil {
		return nil
	}
	c.Header("Retry-After", strconv.FormatInt(max(resetAt-now, 1), 10))
	return types.NewErrorWithStatusCode(
		errors.New(UsageLimitMessage(*blocking, modelName, resetAt, now)),
		types.ErrorCodeUserUsageLimitReached,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithNoRecordErrorLog(),
	)
}

// UsageLimitMessage tells an API client which limit it reached and when it
// resets, in the server's time zone. API clients get English whatever the
// console language is. Waiting times round up so they never suggest retrying
// early.
func UsageLimitMessage(rule model.UserUsageRule, modelName string, resetAt int64, now int64) string {
	var limit string
	switch {
	case rule.IsCalendar():
		limit = map[string]string{"day": "daily", "week": "weekly", "month": "monthly", "quarter": "quarterly", "year": "yearly"}[rule.CalendarPeriod]
	case rule.WindowSeconds%86400 == 0:
		limit = fmt.Sprintf("%d-day", rule.WindowSeconds/86400)
	case rule.WindowSeconds%3600 == 0:
		limit = fmt.Sprintf("%d-hour", rule.WindowSeconds/3600)
	default:
		limit = fmt.Sprintf("%d-minute", rule.WindowSeconds/60)
	}
	scope := ""
	if len(rule.Models) > 0 && modelName != "" {
		scope = " for " + modelName
	}

	seconds := max(resetAt-now, 1)
	var days, hours, minutes int64
	if seconds >= 86400 {
		hours = (seconds + 3599) / 3600
		days, hours = hours/24, hours%24
	} else {
		minutes = (seconds + 59) / 60
		hours, minutes = minutes/60, minutes%60
	}
	var parts []string
	for _, part := range []struct {
		count int64
		unit  string
	}{{days, "day"}, {hours, "hour"}, {minutes, "minute"}} {
		switch part.count {
		case 0:
		case 1:
			parts = append(parts, "1 "+part.unit)
		default:
			parts = append(parts, fmt.Sprintf("%d %ss", part.count, part.unit))
		}
	}
	// Round before converting: a daylight saving change can move the offset.
	resetTime := time.Unix(resetAt, 0).Add(59 * time.Second).Truncate(time.Minute).In(time.Local).
		Format("2006-01-02 15:04 UTC-07:00")
	return fmt.Sprintf("You have reached your %s usage limit%s. It will reset in %s, at %s. Please retry after that time.",
		limit, scope, strings.Join(parts, " "), resetTime)
}

// RecordUserUsage adds a settled charge to the windows of the user's rules
// that cover modelName. Disabled rules count too, so switching a rule back on
// keeps its window. Failures are logged and never change the charge itself.
func RecordUserUsage(userID int, modelName string, quota int) {
	now := common.GetTimestamp()
	addUserUsage(userID, modelName, int64(quota), now, now)
}

// RefundUserUsage takes back quota charged at chargedAt (Unix seconds). A
// window that opened after the charge keeps its usage.
func RefundUserUsage(userID int, modelName string, quota int, chargedAt int64) {
	addUserUsage(userID, modelName, -int64(quota), chargedAt, common.GetTimestamp())
}

// RecordRelayUserUsage records the settled charge of a relay request.
// Playground requests count toward no usage limit.
func RecordRelayUserUsage(info *relaycommon.RelayInfo, quota int) {
	if info.IsPlayground {
		return
	}
	RecordUserUsage(info.UserId, info.OriginModelName, quota)
}

func addUserUsage(userID int, modelName string, quota int64, chargedAt int64, now int64) {
	if userID <= 0 || quota == 0 {
		return
	}
	snapshot, err := model.GetUserLimitSnapshot(userID)
	if err != nil {
		common.SysError(common.LogText("failed to load the usage limits of user %d: %s", userID, err.Error()))
		return
	}
	for _, rule := range snapshot.Rules {
		if !rule.AppliesTo(modelName) {
			continue
		}
		if err := model.AddUserUsage(userID, rule, quota, chargedAt, now); err != nil {
			common.SysError(common.LogText("failed to record usage of user %d in the %d-second window: %s", userID, rule.WindowSeconds, err.Error()))
		}
	}
}
