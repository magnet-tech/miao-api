package operation_setting

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Model replacement detection asks each monitored model a question with a
// known answer through every channel that serves it, and acts on channels
// whose answers or declared response models suggest a different model.
//
// The whole configuration is one JSON option so a save replaces it at once
// and a running detection reads one consistent snapshot. Its key and field
// names are those of the first fork release, which deployments already hold.
const ModelReplacementSettingOptionKey = "ModelReplacementDetectionSetting"

const (
	MatchModeExact    = "exact"
	MatchModeContains = "contains"
	MatchModeRegex    = "regex"

	// Response model checks compare the model an upstream declares in its
	// reply with the monitored model. Loose ignores case, a provider path and
	// a date suffix; it is the default, also for settings saved before the
	// check existed.
	ResponseModelCheckOff   = "off"
	ResponseModelCheckExact = "exact"
	ResponseModelCheckLoose = "loose"

	MaxDetectionIntervalMinutes = 7 * 24 * 60
	MaxDetectionRounds          = 50
	MaxDetectionTimeoutSeconds  = 600
	MaxDetectionRetryTimes      = 10
	MaxDetectionModels          = 200
	MaxDetectionTextLength      = 4000
	MaxDetectionActions         = 20
	MaxDetectionWindows         = 24
	MaxDetectionRetentionDays   = 365
	// DetectionRoutingLimit bounds action amounts and the priority and weight
	// they produce, so repeated runs cannot overflow them.
	DetectionRoutingLimit  = math.MaxInt32
	maxDetectionNameLength = 64
)

// Actions run on a channel after a detection run, in the configured order.
const (
	DetectionActionAddGroup         = "add_group"
	DetectionActionRemoveGroup      = "remove_group"
	DetectionActionIncreasePriority = "increase_priority"
	DetectionActionDecreasePriority = "decrease_priority"
	DetectionActionSetPriority      = "set_priority"
	DetectionActionIncreaseWeight   = "increase_weight"
	DetectionActionDecreaseWeight   = "decrease_weight"
	DetectionActionSetWeight        = "set_weight"
	DetectionActionSetTag           = "set_tag"
	DetectionActionRemoveTag        = "remove_tag"
	DetectionActionAutoDisable      = "auto_disable"
	DetectionActionManualDisable    = "manual_disable"
)

// DetectionAction is one configured step: Value names a group or tag, Amount
// a priority or weight.
type DetectionAction struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Amount int64  `json:"amount"`
}

type MonitoredModel struct {
	Model              string `json:"model"`
	Question           string `json:"question"`
	ExpectedAnswer     string `json:"expected_answer"`
	MatchMode          string `json:"match_mode"`
	ResponseModelCheck string `json:"response_model_check"`
	RetryTimes         int    `json:"retry_times"`
	// ExcludedChannelIDs lists the channels left out on purpose; storing
	// exclusions keeps channels added later in scope.
	ExcludedChannelIDs []int `json:"excluded_channel_ids"`
}

// IntervalWindow overrides the interval after runs that finish between Start
// (inclusive) and End (exclusive), HH:MM server time. An End before Start
// runs past midnight.
type IntervalWindow struct {
	Start           string `json:"start"`
	End             string `json:"end"`
	IntervalMinutes int    `json:"interval_minutes"`
}

type ModelReplacementSetting struct {
	// Enabled turns on scheduled runs; runs can be started by hand anyway.
	Enabled         bool             `json:"enabled"`
	IntervalMinutes int              `json:"interval_minutes"`
	IntervalWindows []IntervalWindow `json:"interval_windows"`
	RetentionDays   int              `json:"retention_days"`
	Rounds          int              `json:"rounds"`
	Concurrency     int              `json:"concurrency"`
	TimeoutSeconds  int              `json:"timeout_seconds"`
	// Threshold is the share of mismatched rounds, in percent, a model may
	// have before its channel counts as replaced.
	Threshold float64 `json:"threshold"`
	// PassActions run when every model of a channel passed, FailActions when
	// one looks replaced, RequestFailureActions when a request kept failing.
	PassActions           []DetectionAction `json:"pass_actions"`
	FailActions           []DetectionAction `json:"fail_actions"`
	RequestFailureActions []DetectionAction `json:"request_failure_actions"`
	Models                []MonitoredModel  `json:"models"`
}

var modelReplacementSetting atomic.Pointer[ModelReplacementSetting]

func init() {
	defaults := DefaultModelReplacementSetting()
	modelReplacementSetting.Store(&defaults)
}

func DefaultModelReplacementSetting() ModelReplacementSetting {
	return ModelReplacementSetting{
		IntervalMinutes:       10,
		IntervalWindows:       []IntervalWindow{},
		RetentionDays:         7,
		Rounds:                3,
		Concurrency:           3,
		TimeoutSeconds:        60,
		PassActions:           []DetectionAction{},
		FailActions:           []DetectionAction{},
		RequestFailureActions: []DetectionAction{},
		Models:                []MonitoredModel{},
	}
}

// GetModelReplacementSetting returns the current snapshot; treat its slices
// as read-only.
func GetModelReplacementSetting() ModelReplacementSetting {
	return *modelReplacementSetting.Load()
}

func ModelReplacementSettingJSON() string {
	data, err := common.Marshal(modelReplacementSetting.Load())
	if err != nil {
		return "{}"
	}
	return string(data)
}

// LoadModelReplacementSetting applies a stored value; an invalid one keeps
// the previous snapshot.
func LoadModelReplacementSetting(value string) {
	parsed, err := ParseModelReplacementSetting(value)
	if err != nil {
		common.SysError(common.LogText("failed to load model replacement detection settings: %s", err.Error()))
		return
	}
	modelReplacementSetting.Store(&parsed)
}

// ParseModelReplacementSetting decodes and validates a stored or submitted
// configuration; missing fields take their defaults.
func ParseModelReplacementSetting(value string) (ModelReplacementSetting, error) {
	setting := DefaultModelReplacementSetting()
	if strings.TrimSpace(value) != "" {
		if err := common.UnmarshalJsonStr(value, &setting); err != nil {
			return ModelReplacementSetting{}, common.NewMessage("Invalid model replacement detection settings")
		}
	}
	if setting.RetentionDays == 0 {
		setting.RetentionDays = DefaultModelReplacementSetting().RetentionDays
	}
	return setting, setting.validate()
}

func (setting *ModelReplacementSetting) validate() error {
	bounds := []struct {
		value, min, max int
		message         string
	}{
		{setting.IntervalMinutes, 1, MaxDetectionIntervalMinutes, "Detection interval must be between 1 and {{max}} minutes"},
		{setting.Rounds, 1, MaxDetectionRounds, "Test rounds must be between 1 and {{max}}"},
		{setting.Concurrency, 1, MaxChannelTestConcurrency, "Maximum concurrency must be between 1 and {{max}}"},
		{setting.TimeoutSeconds, 1, MaxDetectionTimeoutSeconds, "Timeout must be between 1 and {{max}} seconds"},
		{setting.RetentionDays, 1, MaxDetectionRetentionDays, "History must be kept between 1 and {{max}} days"},
	}
	for _, bound := range bounds {
		if bound.value < bound.min || bound.value > bound.max {
			return common.NewMessage(bound.message, map[string]any{"max": bound.max})
		}
	}
	if setting.Threshold < 0 || setting.Threshold > 100 || math.IsNaN(setting.Threshold) {
		return common.NewMessage("Mismatch threshold must be between 0 and 100")
	}
	if err := setting.validateWindows(); err != nil {
		return err
	}
	for _, actions := range []*[]DetectionAction{&setting.PassActions, &setting.FailActions, &setting.RequestFailureActions} {
		if *actions == nil {
			*actions = []DetectionAction{}
		}
		if len(*actions) > MaxDetectionActions {
			return common.NewMessage("At most {{max}} actions can be configured for each result", map[string]any{"max": MaxDetectionActions})
		}
		for i := range *actions {
			if err := (*actions)[i].normalize(); err != nil {
				return err
			}
		}
	}
	if setting.Models == nil {
		setting.Models = []MonitoredModel{}
	}
	if len(setting.Models) > MaxDetectionModels {
		return common.NewMessage("At most {{max}} models can be detected", map[string]any{"max": MaxDetectionModels})
	}
	seen := make(map[string]bool, len(setting.Models))
	for i := range setting.Models {
		item := &setting.Models[i]
		if err := item.Normalize(); err != nil {
			return err
		}
		if seen[item.Model] {
			return common.NewMessage("Model {{model}} is configured more than once", map[string]any{"model": item.Model})
		}
		seen[item.Model] = true
	}
	return nil
}

// Normalize trims a monitored model, fills in defaults and validates it.
func (item *MonitoredModel) Normalize() error {
	item.Model = strings.TrimSpace(item.Model)
	item.Question = strings.TrimSpace(item.Question)
	item.ExpectedAnswer = strings.TrimSpace(item.ExpectedAnswer)
	if item.Model == "" {
		return common.NewMessage("Model name is required")
	}
	if item.Question == "" || item.ExpectedAnswer == "" {
		return common.NewMessage("Enter the test question and expected answer for {{model}}", map[string]any{"model": item.Model})
	}
	if len(item.Question) > MaxDetectionTextLength || len(item.ExpectedAnswer) > MaxDetectionTextLength {
		return common.NewMessage("The test question and expected answer for {{model}} must not exceed {{max}} characters", map[string]any{"model": item.Model, "max": MaxDetectionTextLength})
	}
	switch item.MatchMode {
	case "":
		item.MatchMode = MatchModeExact
	case MatchModeExact, MatchModeContains:
	case MatchModeRegex:
		if _, err := regexp.Compile(item.ExpectedAnswer); err != nil {
			return common.NewMessage("The expected answer for {{model}} is not a valid regular expression", map[string]any{"model": item.Model})
		}
	default:
		return common.NewMessage("Unknown answer match mode for {{model}}", map[string]any{"model": item.Model})
	}
	switch item.ResponseModelCheck {
	case "":
		item.ResponseModelCheck = ResponseModelCheckLoose
	case ResponseModelCheckOff, ResponseModelCheckExact, ResponseModelCheckLoose:
	default:
		return common.NewMessage("Unknown response model check for {{model}}", map[string]any{"model": item.Model})
	}
	if item.RetryTimes < 0 || item.RetryTimes > MaxDetectionRetryTimes {
		return common.NewMessage("Retry times for {{model}} must be between 0 and {{max}}", map[string]any{"model": item.Model, "max": MaxDetectionRetryTimes})
	}
	item.ExcludedChannelIDs = slices.Compact(slices.Sorted(slices.Values(item.ExcludedChannelIDs)))
	if item.ExcludedChannelIDs == nil {
		item.ExcludedChannelIDs = []int{}
	}
	return nil
}

// AnswerMatches reports whether a reply, without surrounding whitespace,
// satisfies the expected answer. Exact and contains are case-sensitive.
func (item MonitoredModel) AnswerMatches(answer string) bool {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return false
	}
	switch item.MatchMode {
	case MatchModeContains:
		return strings.Contains(answer, item.ExpectedAnswer)
	case MatchModeRegex:
		pattern, err := regexp.Compile(item.ExpectedAnswer)
		return err == nil && pattern.MatchString(answer)
	default:
		return answer == item.ExpectedAnswer
	}
}

// Response model notes explain how a declared response model was judged.
const (
	ResponseModelNotChecked    = "not_checked"
	ResponseModelExact         = "exact"
	ResponseModelLooseAccepted = "loose_accepted"
	ResponseModelMismatch      = "mismatch"
)

var datedModelSuffix = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8})$`)

// looseModelName drops case, a provider path and a date suffix:
// "openai/GPT-5.5-2026-11-01" becomes "gpt-5.5".
func looseModelName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if index := strings.LastIndex(name, "/"); index >= 0 {
		name = name[index+1:]
	}
	return datedModelSuffix.ReplaceAllString(name, "")
}

// CheckResponseModel judges the model an upstream declared. Besides the
// monitored model it accepts the name the channel maps it to, which the
// administrator chose. An upstream that declares nothing is not checked.
func (item MonitoredModel) CheckResponseModel(declared string, mapped string) (bool, string) {
	declared = strings.TrimSpace(declared)
	if item.ResponseModelCheck == ResponseModelCheckOff || declared == "" {
		return true, ResponseModelNotChecked
	}
	if declared == item.Model || (mapped != "" && declared == mapped) {
		return true, ResponseModelExact
	}
	if item.ResponseModelCheck == ResponseModelCheckLoose {
		loose := looseModelName(declared)
		if loose == looseModelName(item.Model) || (mapped != "" && loose == looseModelName(mapped)) {
			return true, ResponseModelLooseAccepted
		}
	}
	return false, ResponseModelMismatch
}

func (action *DetectionAction) normalize() error {
	action.Value = strings.TrimSpace(action.Value)
	switch action.Type {
	case DetectionActionAddGroup, DetectionActionRemoveGroup:
		action.Amount = 0
		if action.Value == "" || strings.Contains(action.Value, ",") || len(action.Value) > maxDetectionNameLength {
			return common.NewMessage("Group names cannot be empty, contain commas, or exceed 64 characters")
		}
	case DetectionActionSetTag, DetectionActionRemoveTag:
		action.Amount = 0
		if (action.Type == DetectionActionSetTag && action.Value == "") || len(action.Value) > maxDetectionNameLength {
			return common.NewMessage("Tags cannot be empty or exceed 64 characters")
		}
	case DetectionActionIncreasePriority, DetectionActionDecreasePriority, DetectionActionIncreaseWeight, DetectionActionDecreaseWeight:
		action.Value = ""
		if action.Amount < 1 || action.Amount > DetectionRoutingLimit {
			return common.NewMessage("Amounts must be between 1 and {{max}}", map[string]any{"max": DetectionRoutingLimit})
		}
	case DetectionActionSetPriority:
		action.Value = ""
		if action.Amount < -DetectionRoutingLimit || action.Amount > DetectionRoutingLimit {
			return common.NewMessage("Priority must be between {{min}} and {{max}}", map[string]any{"min": -DetectionRoutingLimit, "max": DetectionRoutingLimit})
		}
	case DetectionActionSetWeight:
		action.Value = ""
		if action.Amount < 0 || action.Amount > DetectionRoutingLimit {
			return common.NewMessage("Weight must be between 0 and {{max}}", map[string]any{"max": DetectionRoutingLimit})
		}
	case DetectionActionAutoDisable, DetectionActionManualDisable:
		action.Value, action.Amount = "", 0
	default:
		return common.NewMessage("Unknown detection action")
	}
	return nil
}

// clockMinutes parses HH:MM into minutes after midnight.
func clockMinutes(value string) (int, bool) {
	hours, minutes, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || len(hours) != 2 || len(minutes) != 2 {
		return 0, false
	}
	hour, hourErr := strconv.Atoi(hours)
	minute, minuteErr := strconv.Atoi(minutes)
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// validateWindows rejects malformed or overlapping windows and sorts them by
// start time.
func (setting *ModelReplacementSetting) validateWindows() error {
	if setting.IntervalWindows == nil {
		setting.IntervalWindows = []IntervalWindow{}
	}
	if len(setting.IntervalWindows) > MaxDetectionWindows {
		return common.NewMessage("At most {{max}} time windows can be configured", map[string]any{"max": MaxDetectionWindows})
	}
	// Each window covers one or, across midnight, two day segments.
	type segment struct{ window, start, end int }
	var segments []segment
	for i := range setting.IntervalWindows {
		window := &setting.IntervalWindows[i]
		window.Start, window.End = strings.TrimSpace(window.Start), strings.TrimSpace(window.End)
		start, startOK := clockMinutes(window.Start)
		end, endOK := clockMinutes(window.End)
		if !startOK || !endOK {
			return common.NewMessage("Time windows must use 24-hour HH:MM times")
		}
		if start == end {
			return common.NewMessage("A time window must not start and end at the same time")
		}
		if window.IntervalMinutes < 1 || window.IntervalMinutes > MaxDetectionIntervalMinutes {
			return common.NewMessage("Detection interval must be between 1 and {{max}} minutes", map[string]any{"max": MaxDetectionIntervalMinutes})
		}
		if start < end {
			segments = append(segments, segment{i, start, end})
		} else {
			segments = append(segments, segment{i, start, 24 * 60}, segment{i, 0, end})
		}
	}
	for i, a := range segments {
		for _, b := range segments[i+1:] {
			if a.window != b.window && a.start < b.end && b.start < a.end {
				first, second := setting.IntervalWindows[a.window], setting.IntervalWindows[b.window]
				return common.NewMessage("Time windows {{first}} and {{second}} overlap", map[string]any{
					"first": first.Start + "-" + first.End, "second": second.Start + "-" + second.End,
				})
			}
		}
	}
	slices.SortFunc(setting.IntervalWindows, func(a, b IntervalWindow) int { return strings.Compare(a.Start, b.Start) })
	return nil
}

// IntervalAt is the wait after a run that finished at the given time: the
// interval of the window containing its server-local time of day, or the
// default interval.
func (setting ModelReplacementSetting) IntervalAt(finishedAt time.Time) time.Duration {
	local := finishedAt.Local()
	minute := local.Hour()*60 + local.Minute()
	for _, window := range setting.IntervalWindows {
		start, _ := clockMinutes(window.Start)
		end, _ := clockMinutes(window.End)
		inside := minute >= start && minute < end
		if start > end {
			inside = minute >= start || minute < end
		}
		if inside {
			return time.Duration(window.IntervalMinutes) * time.Minute
		}
	}
	return time.Duration(setting.IntervalMinutes) * time.Minute
}
