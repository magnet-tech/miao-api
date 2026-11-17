package controller

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUserLimitTest(t *testing.T) (*model.User, service.AuthIdentity, *model.User) {
	t.Helper()
	operator, identity, target := setupAdminUserTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.UserLimitPolicy{}, &model.UserUsageWindow{}, &model.CasbinRule{}, &model.AuthzRole{}))
	common.IsMasterNode = true
	require.NoError(t, authz.Init(model.DB))
	t.Cleanup(func() { model.InvalidateUserLimitCache(target.Id) })
	return operator, identity, target
}

type limitsResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    userLimitsView `json:"data"`
}

func callLimits(t *testing.T, identity service.AuthIdentity, role int, method string, userID int, path, body string, handler gin.HandlerFunc) (int, limitsResponse) {
	t.Helper()
	recorder := adminUserRequest(method, path, body, "", identity, role, idParams(userID), handler)
	var response limitsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
}

func TestUserLimitsAPI(t *testing.T) {
	_, identity, target := setupUserLimitTest(t)
	const rules = `{"max_concurrency":3,"rules":[
		{"window_seconds":18000,"mode":"window","calendar_period":"","quota_limit":500,"enabled":true,"models":[" gpt-4o ","claude","gpt-4o"]},
		{"window_seconds":0,"mode":"calendar","calendar_period":"week","quota_limit":900,"enabled":false,"models":[]}]}`
	_, saved := callLimits(t, identity, common.RoleRootUser, http.MethodPut, target.Id, "/api/user/:id/limits", rules, UpdateUserLimits)
	require.True(t, saved.Success, saved.Message)
	assert.Equal(t, 3, saved.Data.MaxConcurrency)
	require.Len(t, saved.Data.Rules, 2)
	assert.Equal(t, usageRuleView{WindowSeconds: 18000, Mode: "window", QuotaLimit: 500, Enabled: true, Models: []string{"claude", "gpt-4o"}, State: "idle"}, saved.Data.Rules[0])
	assert.Equal(t, "calendar", saved.Data.Rules[1].Mode)
	assert.Equal(t, int64(7*86400), saved.Data.Rules[1].WindowSeconds, "a calendar rule is keyed by its period")

	var stored model.UserLimitPolicy
	require.NoError(t, model.DB.Where("user_id = ?", target.Id).First(&stored).Error)
	assert.JSONEq(t, `[{"window_seconds":18000,"quota_limit":500,"enabled":true,"models":["claude","gpt-4o"]},
		{"window_seconds":604800,"quota_limit":900,"enabled":false,"mode":"calendar","calendar_period":"week"}]`, stored.UsageRules,
		"rules are stored in the first fork release's format")

	service.RecordUserUsage(target.Id, "gpt-4o", 500)
	_, current := callLimits(t, identity, common.RoleRootUser, http.MethodGet, target.Id, "/api/user/:id/limits", "", GetUserLimits)
	require.True(t, current.Success)
	assert.Equal(t, "exhausted", current.Data.Rules[0].State)
	assert.Equal(t, int64(500), current.Data.Rules[0].UsedQuota)
	assert.InDelta(t, time.Now().Unix()+18000, current.Data.Rules[0].ResetAt, 2)
	assert.Equal(t, int64(500), current.Data.Rules[1].UsedQuota, "disabled rules keep counting")

	// The user sees only enabled rules.
	self := adminUserRequest(http.MethodGet, "/api/user/self/limits", "", "", service.AuthIdentity{UserID: target.Id}, common.RoleCommonUser, nil, GetSelfLimits)
	var own limitsResponse
	require.NoError(t, common.Unmarshal(self.Body.Bytes(), &own))
	require.Len(t, own.Data.Rules, 1)
	assert.Equal(t, int64(18000), own.Data.Rules[0].WindowSeconds)

	_, reset := callLimits(t, identity, common.RoleRootUser, http.MethodPost, target.Id, "/api/user/:id/limits/reset", `{"window_seconds":18000}`, ResetUserLimitWindows)
	require.True(t, reset.Success)
	assert.Equal(t, "idle", reset.Data.Rules[0].State)
	assert.Equal(t, int64(500), reset.Data.Rules[1].UsedQuota, "only the named window ends")

	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("action IN ?", []string{"user.limits_update", "user.limits_reset"}).Find(&audits).Error)
	assert.Len(t, audits, 2)
}

func TestUserLimitsRejectInvalidRules(t *testing.T) {
	_, identity, target := setupUserLimitTest(t)
	for name, body := range map[string]string{
		"concurrency above the cap": `{"max_concurrency":1001,"rules":[]}`,
		"negative concurrency":      `{"max_concurrency":-1,"rules":[]}`,
		"window under a minute":     `{"rules":[{"window_seconds":30,"quota_limit":1,"enabled":true}]}`,
		"window over 90 days":       `{"rules":[{"window_seconds":7776060,"quota_limit":1,"enabled":true}]}`,
		"window in partial minutes": `{"rules":[{"window_seconds":90,"quota_limit":1,"enabled":true}]}`,
		"unknown calendar period":   `{"rules":[{"mode":"calendar","calendar_period":"decade","quota_limit":1,"enabled":true}]}`,
		"period on a window rule":   `{"rules":[{"window_seconds":3600,"calendar_period":"day","quota_limit":1,"enabled":true}]}`,
		"unknown mode":              `{"rules":[{"mode":"sliding","window_seconds":3600,"quota_limit":1,"enabled":true}]}`,
		"zero quota":                `{"rules":[{"window_seconds":3600,"quota_limit":0,"enabled":true}]}`,
		"day window and day period": `{"rules":[{"window_seconds":86400,"quota_limit":1,"enabled":true},{"mode":"calendar","calendar_period":"day","quota_limit":1,"enabled":true}]}`,
		"blank model":               `{"rules":[{"window_seconds":3600,"quota_limit":1,"enabled":true,"models":[" "]}]}`,
		"more than ten rules":       fmt.Sprintf(`{"rules":[%s{"window_seconds":60,"quota_limit":1,"enabled":true}]}`, repeatRules(10)),
		"malformed body":            `{"rules":"all"}`,
	} {
		_, response := callLimits(t, identity, common.RoleRootUser, http.MethodPut, target.Id, "/api/user/:id/limits", body, UpdateUserLimits)
		assert.False(t, response.Success, name)
	}
	var policies int64
	require.NoError(t, model.DB.Model(&model.UserLimitPolicy{}).Where("user_id = ?", target.Id).Count(&policies).Error)
	assert.Zero(t, policies)
}

func repeatRules(count int) string {
	var rules string
	for i := range count {
		rules += fmt.Sprintf(`{"window_seconds":%d,"quota_limit":1,"enabled":true},`, (i+2)*60)
	}
	return rules
}

func TestUserLimitsFollowRoleAndPermission(t *testing.T) {
	operator, identity, target := setupUserLimitTest(t)
	require.NoError(t, model.DB.Model(operator).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))
	peer := &model.User{Username: "peer-admin", Password: "x", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "peer-aff"}
	require.NoError(t, model.DB.Create(peer).Error)
	t.Cleanup(func() { model.InvalidateUserLimitCache(peer.Id) })
	body := `{"max_concurrency":1,"rules":[]}`

	code, _ := callLimits(t, identity, common.RoleAdminUser, http.MethodPut, peer.Id, "/api/user/:id/limits", body, UpdateUserLimits)
	assert.Equal(t, http.StatusForbidden, code, "administrators limit common users only")
	_, saved := callLimits(t, identity, common.RoleAdminUser, http.MethodPut, target.Id, "/api/user/:id/limits", body, UpdateUserLimits)
	assert.True(t, saved.Success)

	require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{authz.ResourceUserLimit: {authz.ActionWrite: false, authz.ActionReset: false}}))
	code, _ = callLimits(t, identity, common.RoleAdminUser, http.MethodPut, target.Id, "/api/user/:id/limits", body, UpdateUserLimits)
	assert.Equal(t, http.StatusForbidden, code)
	code, _ = callLimits(t, identity, common.RoleAdminUser, http.MethodPost, target.Id, "/api/user/:id/limits/reset", `{}`, ResetUserLimitWindows)
	assert.Equal(t, http.StatusForbidden, code)
	_, view := callLimits(t, identity, common.RoleAdminUser, http.MethodGet, target.Id, "/api/user/:id/limits", "", GetUserLimits)
	assert.True(t, view.Success, "reading needs only the read permission")
}

func TestUsageLimitOverview(t *testing.T) {
	operator, identity, target := setupUserLimitTest(t)
	near := &model.User{Username: "near-user", Password: "x", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "near-aff"}
	admin := &model.User{Username: "limited-admin", Password: "x", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AffCode: "admin-aff"}
	require.NoError(t, model.DB.Create(near).Error)
	require.NoError(t, model.DB.Create(admin).Error)
	rule := []model.UserUsageRule{{WindowSeconds: 3600, QuotaLimit: 100, Enabled: true}}
	for _, user := range []*model.User{target, near, admin} {
		require.NoError(t, model.SaveUserLimitPolicy(user.Id, 0, rule))
		t.Cleanup(func() { model.InvalidateUserLimitCache(user.Id) })
	}
	service.RecordUserUsage(target.Id, "gpt-4o", 100)
	service.RecordUserUsage(near.Id, "gpt-4o", 85)

	overview := func(role int, query string) (map[string]int, []int) {
		recorder := adminUserRequest(http.MethodGet, "/api/usage_limits/users?"+query, "", "", identity, role, nil, GetUsageLimitOverview)
		var response struct {
			Data struct {
				Items  []limitOverviewItem `json:"items"`
				Counts map[string]int      `json:"counts"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
		ids := []int{}
		for _, item := range response.Data.Items {
			ids = append(ids, item.Id)
		}
		return response.Data.Counts, ids
	}
	counts, ids := overview(common.RoleRootUser, "")
	assert.Equal(t, map[string]int{"all": 3, "near": 1, "exhausted": 1}, counts)
	assert.Equal(t, []int{admin.Id, near.Id, target.Id}, ids, "sorted by ID, newest first")
	_, ids = overview(common.RoleRootUser, "filter=exhausted")
	assert.Equal(t, []int{target.Id}, ids)
	_, ids = overview(common.RoleRootUser, "filter=near&keyword=NEAR")
	assert.Equal(t, []int{near.Id}, ids)

	require.NoError(t, model.DB.Model(operator).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))
	counts, ids = overview(common.RoleAdminUser, "")
	assert.Equal(t, 2, counts["all"], "administrators see the users they manage")
	assert.Equal(t, []int{near.Id, target.Id}, ids)
}
