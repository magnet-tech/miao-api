package controller

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

const (
	usageStateIdle      = "idle"
	usageStateActive    = "active"
	usageStateExhausted = "exhausted"

	// A user is near a limit from this share of a window on.
	usageNearPercent = 80
)

// usageRuleView is a rule with its window as of one moment. An idle rolling
// window has not opened yet; an idle calendar period has no usage.
type usageRuleView struct {
	WindowSeconds  int64    `json:"window_seconds"`
	Mode           string   `json:"mode"`
	CalendarPeriod string   `json:"calendar_period"`
	QuotaLimit     int64    `json:"quota_limit"`
	Enabled        bool     `json:"enabled"`
	Models         []string `json:"models"`
	State          string   `json:"state"`
	UsedQuota      int64    `json:"used_quota"`
	WindowStart    int64    `json:"window_start"`
	ResetAt        int64    `json:"reset_at"`
}

type userLimitsView struct {
	MaxConcurrency int             `json:"max_concurrency"`
	InFlight       int             `json:"in_flight"`
	ServerTimeZone string          `json:"server_time_zone"`
	Rules          []usageRuleView `json:"rules"`
}

// highestPercent is the largest share of a window used by an enabled rule.
func (view userLimitsView) highestPercent() float64 {
	highest := 0.0
	for _, rule := range view.Rules {
		if rule.Enabled && rule.QuotaLimit > 0 {
			highest = max(highest, float64(rule.UsedQuota)*100/float64(rule.QuotaLimit))
		}
	}
	return highest
}

func serverTimeZone() string {
	return time.Now().Format("UTC-07:00")
}

func buildUserLimitsView(userID int, snapshot model.UserLimitSnapshot, windows map[int64]*model.UserUsageWindow, enabledOnly bool) userLimitsView {
	now := common.GetTimestamp()
	view := userLimitsView{MaxConcurrency: snapshot.MaxConcurrency, ServerTimeZone: serverTimeZone(), Rules: []usageRuleView{}}
	for _, rule := range snapshot.Rules {
		if enabledOnly && !rule.Enabled {
			continue
		}
		state := rule.State(windows[rule.WindowSeconds], now)
		item := usageRuleView{
			WindowSeconds: rule.WindowSeconds, Mode: model.UsageRuleModeWindow, CalendarPeriod: rule.CalendarPeriod,
			QuotaLimit: rule.QuotaLimit, Enabled: rule.Enabled, Models: rule.Models,
			State: usageStateIdle, UsedQuota: state.UsedQuota, WindowStart: state.WindowStart, ResetAt: state.ResetAt,
		}
		if rule.IsCalendar() {
			item.Mode = model.UsageRuleModeCalendar
		}
		if item.Models == nil {
			item.Models = []string{}
		}
		if state.Active {
			item.State = usageStateActive
			if state.UsedQuota >= rule.QuotaLimit {
				item.State = usageStateExhausted
			}
		}
		view.Rules = append(view.Rules, item)
	}
	if snapshot.MaxConcurrency > 0 {
		inFlight, err := service.UserConcurrencyInFlight(userID)
		if err != nil {
			common.SysError(common.LogText("failed to count the running requests of user %d: %s", userID, err.Error()))
		}
		view.InFlight = inFlight
	}
	return view
}

func loadUserLimitsView(userID int, enabledOnly bool) (userLimitsView, error) {
	snapshot, err := model.LoadUserLimitSnapshot(userID)
	if err != nil {
		return userLimitsView{}, err
	}
	windows, err := model.GetUserUsageWindows(userID)
	if err != nil {
		return userLimitsView{}, err
	}
	return buildUserLimitsView(userID, snapshot, windows, enabledOnly), nil
}

// managedUser resolves the :id user for an operator that holds permission
// and may manage that user, answering the request itself otherwise.
func managedUser(c *gin.Context, permission authz.Permission) (*model.User, bool) {
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), permission) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return nil, false
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return nil, false
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		common.ApiErrorT(c, "User does not exist")
		return nil, false
	}
	if !canManageTargetRole(c.GetInt("role"), user.Role) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("No permission to access users of same or higher level"))
		return nil, false
	}
	return user, true
}

func GetUserLimits(c *gin.Context) {
	user, ok := managedUser(c, authz.UserLimitRead)
	if !ok {
		return
	}
	view, err := loadUserLimitsView(user.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// GetSelfLimits shows users the enabled limits an administrator set for them.
func GetSelfLimits(c *gin.Context) {
	view, err := loadUserLimitsView(c.GetInt("id"), true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

type userLimitsRequest struct {
	MaxConcurrency int                   `json:"max_concurrency"`
	Rules          []model.UserUsageRule `json:"rules"`
}

// normalizeUsageRules checks the rules an administrator submitted and stores
// them the way the first fork release did: window rules carry no mode, model
// names are trimmed, sorted and unique. It returns the message of the first
// problem it finds.
func normalizeUsageRules(rules []model.UserUsageRule) (*common.Message, []model.UserUsageRule) {
	if len(rules) > model.MaxUserUsageRules {
		return common.NewMessage("At most {{count}} usage limit rules are allowed", map[string]any{"count": model.MaxUserUsageRules}), nil
	}
	seen := make(map[int64]bool, len(rules))
	normalized := make([]model.UserUsageRule, 0, len(rules))
	for _, rule := range rules {
		switch rule.Mode {
		case "", model.UsageRuleModeWindow:
			rule.Mode = ""
			if rule.CalendarPeriod != "" {
				return common.NewMessage("Invalid usage limit mode or calendar period"), nil
			}
			if rule.WindowSeconds < model.MinUserUsageWindow || rule.WindowSeconds > model.MaxUserUsageWindow || rule.WindowSeconds%60 != 0 {
				return common.NewMessage("Usage limit windows must be whole minutes between 1 minute and 90 days"), nil
			}
		case model.UsageRuleModeCalendar:
			seconds, ok := model.CalendarPeriodSeconds[rule.CalendarPeriod]
			if !ok {
				return common.NewMessage("Invalid usage limit mode or calendar period"), nil
			}
			rule.WindowSeconds = seconds
		default:
			return common.NewMessage("Invalid usage limit mode or calendar period"), nil
		}
		if rule.QuotaLimit <= 0 || rule.QuotaLimit > common.MaxWalletQuota {
			return common.NewMessage("Usage limit quota is out of range"), nil
		}
		if seen[rule.WindowSeconds] {
			return common.NewMessage("Each usage limit window can only be used once"), nil
		}
		seen[rule.WindowSeconds] = true
		models := make([]string, 0, len(rule.Models))
		for _, name := range rule.Models {
			name = strings.TrimSpace(name)
			if name == "" || len(name) > 512 {
				return common.NewMessage("Invalid model selection"), nil
			}
			models = append(models, name)
		}
		slices.Sort(models)
		rule.Models = slices.Compact(models)
		if len(rule.Models) == 0 {
			rule.Models = nil
		}
		normalized = append(normalized, rule)
	}
	return nil, normalized
}

// UpdateUserLimits replaces a user's rules and concurrency cap. A rule whose
// window and models are unchanged keeps its current window.
func UpdateUserLimits(c *gin.Context) {
	user, ok := managedUser(c, authz.UserLimitWrite)
	if !ok {
		return
	}
	var request userLimitsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if request.MaxConcurrency < 0 || request.MaxConcurrency > model.MaxUserConcurrencyLimit {
		common.ApiErrorT(c, "Maximum concurrency must be between 0 and {{max}}", map[string]any{"max": model.MaxUserConcurrencyLimit})
		return
	}
	problem, rules := normalizeUsageRules(request.Rules)
	if problem != nil {
		common.ApiError(c, problem)
		return
	}
	before, err := model.LoadUserLimitSnapshot(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SaveUserLimitPolicy(user.Id, request.MaxConcurrency, rules); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "user.limits_update", map[string]any{
		"target_username":        user.Username,
		"max_concurrency_before": before.MaxConcurrency,
		"max_concurrency":        request.MaxConcurrency,
		"usage_rules_before":     before.Rules,
		"usage_rules":            rules,
	})
	view, err := loadUserLimitsView(user.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

// ResetUserLimitWindows ends the current window of one rule, or of every rule
// when window_seconds is 0.
func ResetUserLimitWindows(c *gin.Context) {
	user, ok := managedUser(c, authz.UserLimitReset)
	if !ok {
		return
	}
	var request struct {
		WindowSeconds int64 `json:"window_seconds"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.WindowSeconds < 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if err := model.ResetUserUsageWindows(user.Id, request.WindowSeconds); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAuditFor(c, user.Id, "user.limits_reset", map[string]any{
		"target_username": user.Username,
		"window_seconds":  request.WindowSeconds,
	})
	view, err := loadUserLimitsView(user.Id, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

type limitModelGroup struct {
	Vendor string   `json:"vendor"`
	Models []string `json:"models"`
}

// GetUserLimitModels lists the models a rule can name, grouped by vendor: the
// enabled models of every channel and those the user's rules already name.
func GetUserLimitModels(c *gin.Context) {
	user, ok := managedUser(c, authz.UserLimitRead)
	if !ok {
		return
	}
	names := model.GetEnabledModels()
	snapshot, err := model.LoadUserLimitSnapshot(user.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, rule := range snapshot.Rules {
		names = append(names, rule.Models...)
	}
	slices.Sort(names)
	names = slices.Compact(names)

	var metadata []model.Model
	if err := model.DB.Select("model_name", "vendor_id").Where("name_rule = ? AND model_name IN ?", model.NameRuleExact, names).Find(&metadata).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var vendors []model.Vendor
	if err := model.DB.Select("id", "name").Find(&vendors).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	vendorNames := make(map[int]string, len(vendors))
	for _, vendor := range vendors {
		vendorNames[vendor.Id] = vendor.Name
	}
	vendorOf := make(map[string]string, len(metadata))
	for _, item := range metadata {
		vendorOf[item.ModelName] = vendorNames[item.VendorID]
	}
	groups := []limitModelGroup{}
	index := map[string]int{}
	for _, name := range names {
		vendor := vendorOf[name]
		position, exists := index[vendor]
		if !exists {
			position = len(groups)
			index[vendor] = position
			groups = append(groups, limitModelGroup{Vendor: vendor})
		}
		groups[position].Models = append(groups[position].Models, name)
	}
	// Named vendors first, alphabetically; models without one last.
	slices.SortStableFunc(groups, func(a, b limitModelGroup) int {
		if (a.Vendor == "") != (b.Vendor == "") {
			if a.Vendor == "" {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Vendor, b.Vendor)
	})
	common.ApiSuccess(c, groups)
}

type limitOverviewItem struct {
	userLimitsView
	Id             int     `json:"id"`
	Username       string  `json:"username"`
	DisplayName    string  `json:"display_name"`
	Status         int     `json:"status"`
	HighestPercent float64 `json:"highest_percent"`
}

// GetUsageLimitOverview lists the users with limits the operator may manage,
// with how much of their windows they used. filter narrows to users near
// (from 80%) or at a limit.
func GetUsageLimitOverview(c *gin.Context) {
	if !authz.Can(c.GetInt("id"), c.GetInt("role"), authz.UserLimitRead) {
		common.ApiErrorStatus(c, http.StatusForbidden, common.NewMessage("Unauthorized, insufficient privileges"))
		return
	}
	var policies []model.UserLimitPolicy
	if err := model.DB.Find(&policies).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	userIDs := make([]int, 0, len(policies))
	for _, policy := range policies {
		userIDs = append(userIDs, policy.UserId)
	}
	var users []model.User
	if err := model.DB.Select("id", "username", "display_name", "email", "role", "status").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var windows []model.UserUsageWindow
	if err := model.DB.Where("user_id IN ?", userIDs).Find(&windows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	windowsOf := make(map[int]map[int64]*model.UserUsageWindow, len(policies))
	for i := range windows {
		if windowsOf[windows[i].UserId] == nil {
			windowsOf[windows[i].UserId] = map[int64]*model.UserUsageWindow{}
		}
		windowsOf[windows[i].UserId][windows[i].WindowSeconds] = &windows[i]
	}
	usersByID := make(map[int]model.User, len(users))
	for _, user := range users {
		usersByID[user.Id] = user
	}

	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	filter := c.Query("filter")
	role := c.GetInt("role")
	items := []limitOverviewItem{}
	counts := map[string]int{"all": 0, "near": 0, "exhausted": 0}
	for _, policy := range policies {
		user, exists := usersByID[policy.UserId]
		if !exists || !canManageTargetRole(role, user.Role) {
			continue
		}
		if keyword != "" && strconv.Itoa(user.Id) != keyword &&
			!strings.Contains(strings.ToLower(user.Username), keyword) &&
			!strings.Contains(strings.ToLower(user.DisplayName), keyword) &&
			!strings.Contains(strings.ToLower(user.Email), keyword) {
			continue
		}
		snapshot := model.UserLimitSnapshot{MaxConcurrency: policy.MaxConcurrency}
		if policy.UsageRules != "" {
			if err := common.UnmarshalJsonStr(policy.UsageRules, &snapshot.Rules); err != nil {
				common.SysError(common.LogText("invalid usage rules of user %d: %s", user.Id, err.Error()))
			}
		}
		view := buildUserLimitsView(user.Id, snapshot, windowsOf[user.Id], false)
		item := limitOverviewItem{userLimitsView: view, Id: user.Id, Username: user.Username, DisplayName: user.DisplayName, Status: user.Status, HighestPercent: view.highestPercent()}
		exhausted := item.HighestPercent >= 100
		near := !exhausted && item.HighestPercent >= usageNearPercent
		counts["all"]++
		if exhausted {
			counts["exhausted"]++
		}
		if near {
			counts["near"]++
		}
		if (filter == "near" && !near) || (filter == "exhausted" && !exhausted) {
			continue
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b limitOverviewItem) int { return b.Id - a.Id })
	common.ApiSuccess(c, gin.H{"items": items, "counts": counts})
}
