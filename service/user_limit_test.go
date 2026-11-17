package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// limitedUser creates a user whose usage_rules column holds rulesJSON as
// stored by the first fork release, and the given windows.
func limitedUser(t *testing.T, rulesJSON string, windows ...model.UserUsageWindow) int {
	t.Helper()
	user := model.User{Username: "limited", Quota: 1_000_000, Status: common.UserStatusEnabled, Group: "default", AffCode: fmt.Sprintf("limit-%d", time.Now().UnixNano())}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.UserLimitPolicy{UserId: user.Id, UsageRules: rulesJSON}).Error)
	for _, window := range windows {
		window.UserId = user.Id
		require.NoError(t, model.DB.Create(&window).Error)
	}
	t.Cleanup(func() {
		model.DB.Where("user_id = ?", user.Id).Delete(&model.UserUsageWindow{})
		model.DB.Where("user_id = ?", user.Id).Delete(&model.UserLimitPolicy{})
		model.DB.Unscoped().Delete(&user)
		model.InvalidateUserLimitCache(user.Id)
	})
	return user.Id
}

func usageWindow(t *testing.T, userID int, windowSeconds int64) model.UserUsageWindow {
	t.Helper()
	windows, err := model.GetUserUsageWindows(userID)
	require.NoError(t, err)
	if window := windows[windowSeconds]; window != nil {
		return *window
	}
	return model.UserUsageWindow{}
}

func useTimeZone(t *testing.T, offsetHours int) {
	t.Helper()
	previous := time.Local
	time.Local = time.FixedZone(fmt.Sprintf("UTC%+d", offsetHours), offsetHours*3600)
	t.Cleanup(func() { time.Local = previous })
}

func localUnix(value string) int64 {
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, time.Local)
	if err != nil {
		panic(err)
	}
	return parsed.Unix()
}

func TestCheckUserUsageLimit(t *testing.T) {
	now := time.Now().Unix()
	weekly := model.UserUsageRule{Mode: model.UsageRuleModeCalendar, CalendarPeriod: "week"}
	weekStart, weekEnd := weekly.CalendarBounds(now)
	previousWeek, _ := weekly.CalendarBounds(weekStart - 1)

	const fiveHours = `{"window_seconds":18000,"quota_limit":100,"enabled":true}`
	cases := []struct {
		name       string
		rules      string
		windows    []model.UserUsageWindow
		model      string
		pending    int64
		retryAfter int64 // 0: the request passes
		message    string
	}{
		{
			name:       "a spent window blocks until it ends",
			rules:      `[` + fiveHours + `]`,
			windows:    []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 100}},
			model:      "gpt-4o",
			retryAfter: 14400,
			message:    "You have reached your 5-hour usage limit. It will reset in 4 hours, at ",
		},
		{
			name:    "usage below the limit passes",
			rules:   `[` + fiveHours + `]`,
			windows: []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 99}},
			model:   "gpt-4o",
		},
		{
			name:    "an ended window no longer counts",
			rules:   `[` + fiveHours + `]`,
			windows: []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 18000, UsedQuota: 500}},
			model:   "gpt-4o",
		},
		{
			name:    "a disabled rule does not block",
			rules:   `[{"window_seconds":18000,"quota_limit":100,"enabled":false}]`,
			windows: []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 100}},
			model:   "gpt-4o",
		},
		{
			name:    "a model rule leaves other models alone",
			rules:   `[{"window_seconds":18000,"quota_limit":100,"enabled":true,"models":["gpt-4o"]}]`,
			windows: []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 100}},
			model:   "claude-sonnet",
		},
		{
			name:       "a model rule names the model",
			rules:      `[{"window_seconds":18000,"quota_limit":100,"enabled":true,"models":["gpt-4o"]}]`,
			windows:    []model.UserUsageWindow{{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 100}},
			model:      "gpt-4o",
			retryAfter: 14400,
			message:    "You have reached your 5-hour usage limit for gpt-4o.",
		},
		{
			name:       "a calendar week spent this week blocks until Monday",
			rules:      `[{"window_seconds":604800,"quota_limit":50,"enabled":true,"mode":"calendar","calendar_period":"week"}]`,
			windows:    []model.UserUsageWindow{{WindowSeconds: 604800, WindowStart: weekStart, UsedQuota: 50}},
			model:      "gpt-4o",
			retryAfter: weekEnd - now,
			message:    "You have reached your weekly usage limit.",
		},
		{
			name:    "a calendar week spent last week passes",
			rules:   `[{"window_seconds":604800,"quota_limit":50,"enabled":true,"mode":"calendar","calendar_period":"week"}]`,
			windows: []model.UserUsageWindow{{WindowSeconds: 604800, WindowStart: previousWeek, UsedQuota: 50}},
			model:   "gpt-4o",
		},
		{
			name:  "with several windows spent the one resetting last is reported",
			rules: `[` + fiveHours + `,{"window_seconds":86400,"quota_limit":10,"enabled":true}]`,
			windows: []model.UserUsageWindow{
				{WindowSeconds: 18000, WindowStart: now - 3600, UsedQuota: 100},
				{WindowSeconds: 86400, WindowStart: now - 3600, UsedQuota: 10},
			},
			model:      "gpt-4o",
			retryAfter: 82800,
			message:    "You have reached your 1-day usage limit.",
		},
		{
			name:       "pending realtime charges count and open the window",
			rules:      `[` + fiveHours + `]`,
			model:      "gpt-4o-realtime",
			pending:    100,
			retryAfter: 18000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := limitedUser(t, tc.rules, tc.windows...)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)

			apiErr := CheckUserUsageLimit(c, userID, tc.model, tc.pending)
			if tc.retryAfter == 0 {
				assert.Nil(t, apiErr)
				assert.Empty(t, recorder.Header().Get("Retry-After"))
				return
			}
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeUserUsageLimitReached, apiErr.GetErrorCode())
			assert.Contains(t, apiErr.Error(), tc.message)
			retryAfter, err := strconv.ParseInt(recorder.Header().Get("Retry-After"), 10, 64)
			require.NoError(t, err)
			assert.InDelta(t, tc.retryAfter, retryAfter, 2)
		})
	}
}

func TestUsageLimitMessage(t *testing.T) {
	useTimeZone(t, 8)
	now := localUnix("2026-11-06 20:14") + 30
	cases := []struct {
		rule    model.UserUsageRule
		model   string
		resetAt int64
		want    string
	}{
		{
			rule:    model.UserUsageRule{WindowSeconds: 18000},
			resetAt: now + 2*3600 + 15*60 - 10,
			want:    "You have reached your 5-hour usage limit. It will reset in 2 hours 15 minutes, at 2026-11-06 22:30 UTC+08:00. Please retry after that time.",
		},
		{
			rule:    model.UserUsageRule{WindowSeconds: 5400, Models: []string{"gpt-4o"}},
			model:   "gpt-4o",
			resetAt: now + 20,
			want:    "You have reached your 90-minute usage limit for gpt-4o. It will reset in 1 minute, at 2026-11-06 20:15 UTC+08:00. Please retry after that time.",
		},
		{
			rule:    model.UserUsageRule{WindowSeconds: 7 * 86400},
			resetAt: now + 86400 + 2*3600 + 1,
			want:    "You have reached your 7-day usage limit. It will reset in 1 day 3 hours, at 2026-11-07 22:15 UTC+08:00. Please retry after that time.",
		},
		{
			rule:    model.UserUsageRule{WindowSeconds: 30 * 86400, Mode: model.UsageRuleModeCalendar, CalendarPeriod: "month"},
			resetAt: localUnix("2026-12-01 00:00"),
			want:    "You have reached your monthly usage limit. It will reset in 24 days 4 hours, at 2026-12-01 00:00 UTC+08:00. Please retry after that time.",
		},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, UsageLimitMessage(tc.rule, tc.model, tc.resetAt, now))
	}
}

func TestCalendarPeriodsFollowTheServerCalendar(t *testing.T) {
	useTimeZone(t, 8)
	now := localUnix("2026-11-22 23:30") // a Sunday
	cases := map[string][2]string{
		"day":     {"2026-11-22 00:00", "2026-11-23 00:00"},
		"week":    {"2026-11-16 00:00", "2026-11-23 00:00"},
		"month":   {"2026-11-01 00:00", "2026-12-01 00:00"},
		"quarter": {"2026-10-01 00:00", "2027-01-01 00:00"},
		"year":    {"2026-01-01 00:00", "2027-01-01 00:00"},
	}
	for period, want := range cases {
		start, end := model.UserUsageRule{Mode: model.UsageRuleModeCalendar, CalendarPeriod: period}.CalendarBounds(now)
		assert.Equal(t, [2]int64{localUnix(want[0]), localUnix(want[1])}, [2]int64{start, end}, period)
	}
}

func TestUsageWindowsFollowChargesAndRefunds(t *testing.T) {
	useTimeZone(t, 8)
	hourly := model.UserUsageRule{WindowSeconds: 3600, QuotaLimit: 100, Enabled: true}
	daily := model.UserUsageRule{WindowSeconds: 86400, QuotaLimit: 100, Mode: model.UsageRuleModeCalendar, CalendarPeriod: "day"}
	userID := limitedUser(t, `[]`)

	start := localUnix("2026-11-06 23:00")
	steps := []struct {
		name      string
		rule      model.UserUsageRule
		quota     int64
		chargedAt int64
		now       int64
		wantStart int64
		wantUsed  int64
	}{
		{"the first charge opens the window", hourly, 40, start, start, start, 40},
		{"a later charge adds to it", hourly, 30, start + 600, start + 600, start, 70},
		{"a refund of a charge in the window takes it back", hourly, -30, start + 600, start + 900, start, 40},
		{"usage never goes below zero", hourly, -500, start, start + 900, start, 0},
		{"another charge", hourly, 10, start + 1000, start + 1000, start, 10},
		{"a refund after the window ended is dropped", hourly, -10, start + 1000, start + 3600, start, 10},
		{"a charge after the end opens a new window", hourly, 25, start + 4000, start + 4000, start + 4000, 25},
		{"a refund of a charge from an earlier window is dropped", hourly, -25, start + 1000, start + 4100, start + 4000, 25},
		{"a calendar charge counts in the current day", daily, 10, start, start, localUnix("2026-11-06 00:00"), 10},
		{"the next day starts over", daily, 5, start + 5400, start + 5400, localUnix("2026-11-07 00:00"), 5},
		{"a refund of yesterday's charge is dropped", daily, -10, start, start + 7200, localUnix("2026-11-07 00:00"), 5},
		{"a refund of today's charge is taken back", daily, -5, start + 5400, start + 7200, localUnix("2026-11-07 00:00"), 0},
	}
	for _, step := range steps {
		require.NoError(t, model.AddUserUsage(userID, step.rule, step.quota, step.chargedAt, step.now), step.name)
		window := usageWindow(t, userID, step.rule.WindowSeconds)
		assert.Equal(t, [2]int64{step.wantStart, step.wantUsed}, [2]int64{window.WindowStart, window.UsedQuota}, step.name)
	}
}

func TestSavingLimitsKeepsWindowsOfUnchangedRules(t *testing.T) {
	userID := limitedUser(t, `[]`)
	scoped := model.UserUsageRule{WindowSeconds: 3600, QuotaLimit: 100, Enabled: true, Models: []string{"a", "b"}}
	daily := model.UserUsageRule{WindowSeconds: 86400, QuotaLimit: 100, Enabled: true}
	require.NoError(t, model.SaveUserLimitPolicy(userID, 2, []model.UserUsageRule{scoped, daily}))
	RecordUserUsage(userID, "a", 30)
	RecordUserUsage(userID, "c", 7)
	assert.Equal(t, int64(30), usageWindow(t, userID, 3600).UsedQuota)
	assert.Equal(t, int64(37), usageWindow(t, userID, 86400).UsedQuota)

	// A new limit or switch keeps the window; other models start over.
	scoped.Models, scoped.QuotaLimit, scoped.Enabled = []string{"b", "a"}, 500, false
	daily.Models = []string{"c"}
	require.NoError(t, model.SaveUserLimitPolicy(userID, 2, []model.UserUsageRule{scoped, daily}))
	assert.Equal(t, int64(30), usageWindow(t, userID, 3600).UsedQuota)
	assert.Zero(t, usageWindow(t, userID, 86400).UsedQuota)
	snapshot, err := model.GetUserLimitSnapshot(userID)
	require.NoError(t, err)
	assert.Equal(t, model.UserLimitSnapshot{MaxConcurrency: 2, Rules: []model.UserUsageRule{scoped, daily}}, snapshot, "saving refreshes the cached policy")

	RecordUserUsage(userID, "b", 5)
	assert.Equal(t, int64(35), usageWindow(t, userID, 3600).UsedQuota, "disabled rules keep counting")

	require.NoError(t, model.SaveUserLimitPolicy(userID, 0, nil))
	snapshot, err = model.GetUserLimitSnapshot(userID)
	require.NoError(t, err)
	assert.Equal(t, model.UserLimitSnapshot{}, snapshot)
	var policies, windows int64
	require.NoError(t, model.DB.Model(&model.UserLimitPolicy{}).Where("user_id = ?", userID).Count(&policies).Error)
	require.NoError(t, model.DB.Model(&model.UserUsageWindow{}).Where("user_id = ?", userID).Count(&windows).Error)
	assert.Zero(t, policies+windows)
}

func TestRelayBillingAppliesUsageLimits(t *testing.T) {
	userID := limitedUser(t, `[{"window_seconds":3600,"quota_limit":100,"enabled":true}]`,
		model.UserUsageWindow{WindowSeconds: 3600, WindowStart: time.Now().Unix() - 60, UsedQuota: 100})
	token := model.Token{UserId: userID, Key: "usage-limit-test", Name: "limit", RemainQuota: 1_000_000, Status: common.TokenStatusEnabled}
	require.NoError(t, model.DB.Create(&token).Error)
	t.Cleanup(func() { model.DB.Unscoped().Delete(&token) })

	request := func(playground bool) (*relaycommon.RelayInfo, *types.NewAPIError) {
		info := &relaycommon.RelayInfo{UserId: userID, TokenId: token.Id, TokenKey: token.Key, OriginModelName: "gpt-4o", UsingGroup: "default", UserGroup: "default",
			UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ForcePreConsume: true, StartTime: time.Now(), IsPlayground: playground}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return info, PreConsumeBilling(c, 10, info)
	}

	info, apiErr := request(false)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeUserUsageLimitReached, apiErr.GetErrorCode())
	assert.Nil(t, info.Billing, "a blocked request reserves nothing")
	quota, err := model.GetUserQuota(userID, true)
	require.NoError(t, err)
	assert.Equal(t, 1_000_000, quota)

	info, apiErr = request(true)
	require.Nil(t, apiErr, "the playground is not limited")
	require.NotNil(t, info.Billing)
	RecordRelayUserUsage(info, 40)
	assert.Equal(t, int64(100), usageWindow(t, userID, 3600).UsedQuota, "the playground does not count")
	info.IsPlayground = false
	RecordRelayUserUsage(info, 40)
	assert.Equal(t, int64(140), usageWindow(t, userID, 3600).UsedQuota)
}

func TestTaskRefundsReturnUsageToTheirWindow(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID = 1, 1, 1
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-test-key", 5000)
	seedChannel(t, channelID)
	require.NoError(t, model.SaveUserLimitPolicy(userID, 0, []model.UserUsageRule{{WindowSeconds: 3600, QuotaLimit: 5000, Enabled: true}}))
	t.Cleanup(func() {
		require.NoError(t, model.SaveUserLimitPolicy(userID, 0, nil))
	})
	RecordUserUsage(userID, "test-model", 3000)
	windowStart := usageWindow(t, userID, 3600).WindowStart

	earlier := makeTask(userID, channelID, 1000, tokenID, BillingSourceWallet, 0)
	earlier.SubmitTime = windowStart - 10
	require.NoError(t, model.DB.Create(earlier).Error)
	assert.True(t, RefundTaskQuota(context.Background(), earlier, "failed"))
	assert.Equal(t, int64(3000), usageWindow(t, userID, 3600).UsedQuota, "the charge belonged to an earlier window")

	current := makeTask(userID, channelID, 1000, tokenID, BillingSourceWallet, 0)
	current.TaskID += "-current"
	current.SubmitTime = windowStart
	require.NoError(t, model.DB.Create(current).Error)
	assert.True(t, RefundTaskQuota(context.Background(), current, "failed"))
	assert.Equal(t, int64(2000), usageWindow(t, userID, 3600).UsedQuota)
}
