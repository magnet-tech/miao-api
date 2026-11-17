package model

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Per-user usage limits: an administrator caps the quota a user may spend in
// a time window, optionally for some models only, and the number of relay
// requests the user may run at once. The table layout matches the first fork
// release so its stored rules and windows keep working.

const (
	MaxUserUsageRules       = 10
	MaxUserConcurrencyLimit = 1000
	MinUserUsageWindow      = int64(60)
	MaxUserUsageWindow      = int64(90 * 86400)

	UsageRuleModeWindow   = "window"
	UsageRuleModeCalendar = "calendar"

	userLimitCacheTTL = 10 * time.Minute
)

// Calendar periods use fixed identifiers as their window_seconds; the actual
// boundaries follow the server's local calendar.
var CalendarPeriodSeconds = map[string]int64{
	"day":     86400,
	"week":    7 * 86400,
	"month":   30 * 86400,
	"quarter": 90 * 86400,
	"year":    365 * 86400,
}

type UserLimitPolicy struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	UserId         int    `json:"user_id" gorm:"not null;uniqueIndex"`
	MaxConcurrency int    `json:"max_concurrency" gorm:"not null;default:0"`
	UsageRules     string `json:"-" gorm:"type:text"` // JSON []UserUsageRule
	UpdatedAt      int64  `json:"updated_at"`
}

func (UserLimitPolicy) TableName() string {
	return "user_limit_policies"
}

// UserUsageRule caps the spending of the selected models (all when empty) in a
// rolling window that opens at the first charge, or in a calendar period.
// WindowSeconds identifies the rule within a user's policy.
type UserUsageRule struct {
	WindowSeconds  int64    `json:"window_seconds"`
	QuotaLimit     int64    `json:"quota_limit"`
	Enabled        bool     `json:"enabled"`
	Models         []string `json:"models,omitempty"`
	Mode           string   `json:"mode,omitempty"`
	CalendarPeriod string   `json:"calendar_period,omitempty"`
}

func (rule UserUsageRule) IsCalendar() bool {
	return rule.Mode == UsageRuleModeCalendar
}

func (rule UserUsageRule) AppliesTo(modelName string) bool {
	return len(rule.Models) == 0 || slices.Contains(rule.Models, modelName)
}

// SameScope reports whether two rules count the same usage, so a window can
// carry over when only the limit or the switch changes.
func (rule UserUsageRule) SameScope(other UserUsageRule) bool {
	if rule.WindowSeconds != other.WindowSeconds || rule.IsCalendar() != other.IsCalendar() || rule.CalendarPeriod != other.CalendarPeriod {
		return false
	}
	a, b := slices.Clone(rule.Models), slices.Clone(other.Models)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// CalendarBounds returns the server-local calendar period containing now.
// Weeks begin on Monday.
func (rule UserUsageRule) CalendarBounds(now int64) (int64, int64) {
	t := time.Unix(now, 0).In(time.Local)
	year, month, day := t.Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	var end time.Time
	switch rule.CalendarPeriod {
	case "week":
		start = start.AddDate(0, 0, -((int(start.Weekday()) + 6) % 7))
		end = start.AddDate(0, 0, 7)
	case "month":
		start = time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
		end = start.AddDate(0, 1, 0)
	case "quarter":
		start = time.Date(year, time.Month((int(month)-1)/3*3+1), 1, 0, 0, 0, 0, time.Local)
		end = start.AddDate(0, 3, 0)
	case "year":
		start = time.Date(year, time.January, 1, 0, 0, 0, 0, time.Local)
		end = start.AddDate(1, 0, 0)
	default:
		end = start.AddDate(0, 0, 1)
	}
	return start.Unix(), end.Unix()
}

// UserUsageWindow holds one rule's counter. WindowStart 0 means the rolling
// window has not opened; for calendar rules it is the period it counts.
type UserUsageWindow struct {
	Id            int64 `json:"id" gorm:"primaryKey"`
	UserId        int   `json:"user_id" gorm:"not null;uniqueIndex:idx_user_usage_window,priority:1"`
	WindowSeconds int64 `json:"window_seconds" gorm:"not null;uniqueIndex:idx_user_usage_window,priority:2"`
	WindowStart   int64 `json:"window_start" gorm:"not null;default:0"`
	UsedQuota     int64 `json:"used_quota" gorm:"not null;default:0"`
	UpdatedAt     int64 `json:"updated_at"`
}

func (UserUsageWindow) TableName() string {
	return "user_usage_windows"
}

// UsageWindowState is a rule's window as of now.
type UsageWindowState struct {
	Active      bool
	WindowStart int64
	ResetAt     int64
	UsedQuota   int64
}

// State evaluates a stored window (nil when none) for the rule at now.
func (rule UserUsageRule) State(window *UserUsageWindow, now int64) UsageWindowState {
	if rule.IsCalendar() {
		start, end := rule.CalendarBounds(now)
		state := UsageWindowState{WindowStart: start, ResetAt: end}
		if window != nil && window.WindowStart >= start && window.WindowStart < end {
			state.UsedQuota = window.UsedQuota
		}
		state.Active = state.UsedQuota > 0
		return state
	}
	if window == nil || window.WindowStart <= 0 || now >= window.WindowStart+rule.WindowSeconds {
		return UsageWindowState{}
	}
	return UsageWindowState{Active: true, WindowStart: window.WindowStart, ResetAt: window.WindowStart + rule.WindowSeconds, UsedQuota: window.UsedQuota}
}

// UserLimitSnapshot is a user's parsed policy; the zero value means none.
type UserLimitSnapshot struct {
	MaxConcurrency int             `json:"max_concurrency"`
	Rules          []UserUsageRule `json:"usage_rules"`
}

// --- Policy cache -----------------------------------------------------------
// Relay requests read the policy on every request. With Redis every instance
// shares one cached copy that saving deletes; without Redis there is a single
// instance and an in-process copy.

type cachedLimitSnapshot struct {
	snapshot UserLimitSnapshot
	expires  time.Time
}

var (
	userLimitCacheMu sync.Mutex
	userLimitCache   = map[int]cachedLimitSnapshot{}
)

func userLimitCacheKey(userID int) string {
	return fmt.Sprintf("user_limit_policy:%d", userID)
}

// GetUserLimitSnapshot returns the user's policy, cached.
func GetUserLimitSnapshot(userID int) (UserLimitSnapshot, error) {
	if common.RedisEnabled {
		if raw, err := common.RedisGet(userLimitCacheKey(userID)); err == nil {
			var snapshot UserLimitSnapshot
			if common.UnmarshalJsonStr(raw, &snapshot) == nil {
				return snapshot, nil
			}
		}
	} else {
		userLimitCacheMu.Lock()
		cached, ok := userLimitCache[userID]
		userLimitCacheMu.Unlock()
		if ok && time.Now().Before(cached.expires) {
			return cached.snapshot, nil
		}
	}
	snapshot, err := loadUserLimitSnapshot(DB, userID)
	if err != nil {
		return UserLimitSnapshot{}, err
	}
	if common.RedisEnabled {
		if encoded, err := common.Marshal(snapshot); err == nil {
			if err := common.RedisSet(userLimitCacheKey(userID), string(encoded), userLimitCacheTTL); err != nil {
				common.SysLog(common.LogText("failed to cache usage limits of user %d: %s", userID, err.Error()))
			}
		}
	} else {
		userLimitCacheMu.Lock()
		userLimitCache[userID] = cachedLimitSnapshot{snapshot: snapshot, expires: time.Now().Add(userLimitCacheTTL)}
		userLimitCacheMu.Unlock()
	}
	return snapshot, nil
}

// InvalidateUserLimitCache drops the cached policy on every instance.
func InvalidateUserLimitCache(userID int) {
	if common.RedisEnabled {
		if err := common.RedisDel(userLimitCacheKey(userID)); err != nil {
			common.SysLog(common.LogText("failed to clear cached usage limits of user %d: %s", userID, err.Error()))
		}
		return
	}
	userLimitCacheMu.Lock()
	delete(userLimitCache, userID)
	userLimitCacheMu.Unlock()
}

func loadUserLimitSnapshot(tx *gorm.DB, userID int) (UserLimitSnapshot, error) {
	var policy UserLimitPolicy
	err := tx.Where("user_id = ?", userID).Limit(1).Find(&policy).Error
	if err != nil || policy.Id == 0 {
		return UserLimitSnapshot{}, err
	}
	snapshot := UserLimitSnapshot{MaxConcurrency: policy.MaxConcurrency}
	if policy.UsageRules != "" {
		if err := common.UnmarshalJsonStr(policy.UsageRules, &snapshot.Rules); err != nil {
			return UserLimitSnapshot{}, fmt.Errorf("invalid usage rules of user %d: %w", userID, err)
		}
	}
	return snapshot, nil
}

// LoadUserLimitSnapshot reads the stored policy without the cache.
func LoadUserLimitSnapshot(userID int) (UserLimitSnapshot, error) {
	return loadUserLimitSnapshot(DB, userID)
}

// --- Windows -----------------------------------------------------------------

// GetUserUsageWindows returns the user's windows keyed by window_seconds.
func GetUserUsageWindows(userID int) (map[int64]*UserUsageWindow, error) {
	var windows []UserUsageWindow
	if err := DB.Where("user_id = ?", userID).Find(&windows).Error; err != nil {
		return nil, err
	}
	byRule := make(map[int64]*UserUsageWindow, len(windows))
	for i := range windows {
		byRule[windows[i].WindowSeconds] = &windows[i]
	}
	return byRule, nil
}

// AddUserUsage adds quota to the window of rule, or takes it back when quota is
// negative. A charge opens a new window when the previous one ended. A refund
// names when the refunded charge was made (chargedAt) and is dropped unless
// the window that counted that charge is still running. Usage never goes
// below zero.
func AddUserUsage(userID int, rule UserUsageRule, quota int64, chargedAt int64, now int64) error {
	if quota == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&UserUsageWindow{UserId: userID, WindowSeconds: rule.WindowSeconds}).Error; err != nil {
			return err
		}
		var window UserUsageWindow
		if err := lockForUpdate(tx).Where("user_id = ? AND window_seconds = ?", userID, rule.WindowSeconds).
			First(&window).Error; err != nil {
			return err
		}
		state := rule.State(&window, now)
		if quota < 0 && (!state.Active || chargedAt < state.WindowStart) {
			return nil
		}
		start := state.WindowStart
		if !state.Active && !rule.IsCalendar() {
			start = now
		}
		used := max(state.UsedQuota+quota, 0)
		return tx.Model(&UserUsageWindow{}).Where("id = ?", window.Id).
			Updates(map[string]any{"window_start": start, "used_quota": used, "updated_at": now}).Error
	})
}

// ResetUserUsageWindows ends the current window of one rule, or of all rules
// when windowSeconds is 0. Rolling windows reopen at the next charge.
func ResetUserUsageWindows(userID int, windowSeconds int64) error {
	query := DB.Model(&UserUsageWindow{}).Where("user_id = ?", userID)
	if windowSeconds > 0 {
		query = query.Where("window_seconds = ?", windowSeconds)
	}
	return query.Updates(map[string]any{"window_start": 0, "used_quota": 0, "updated_at": common.GetTimestamp()}).Error
}

// SaveUserLimitPolicy replaces a user's policy. A rule whose scope is unchanged
// keeps its window; a removed rule, or one whose models or mode changed,
// starts over. An empty policy is deleted.
func SaveUserLimitPolicy(userID int, maxConcurrency int, rules []UserUsageRule) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		previous, err := loadUserLimitSnapshot(tx, userID)
		if err != nil {
			return err
		}
		keep := make([]int64, 0, len(rules))
		for _, rule := range rules {
			for _, old := range previous.Rules {
				if old.SameScope(rule) {
					keep = append(keep, rule.WindowSeconds)
				}
			}
		}
		stale := tx.Where("user_id = ?", userID)
		if len(keep) > 0 {
			stale = stale.Where("window_seconds NOT IN ?", keep)
		}
		if err := stale.Delete(&UserUsageWindow{}).Error; err != nil {
			return err
		}
		if maxConcurrency == 0 && len(rules) == 0 {
			return tx.Where("user_id = ?", userID).Delete(&UserLimitPolicy{}).Error
		}
		encoded, err := common.Marshal(rules)
		if err != nil {
			return err
		}
		policy := UserLimitPolicy{UserId: userID, MaxConcurrency: maxConcurrency, UsageRules: string(encoded), UpdatedAt: common.GetTimestamp()}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"max_concurrency", "usage_rules", "updated_at"}),
		}).Create(&policy).Error
	})
	if err == nil {
		InvalidateUserLimitCache(userID)
	}
	return err
}
