package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
)

// Model replacement detection runs as a system task: the scheduler starts one
// after the configured wait when detection is on, and administrators start
// one by hand for all channels or retest a single channel at any time.

const (
	detectionStoredTextLimit = 2000
	// legacyResponseModelMismatch is how the first fork release recorded a
	// declared model that differed from the monitored one.
	legacyResponseModelMismatch = "The response model does not exactly match the test model."
)

// detectionDisableReasons are stored as the channel status reason when an
// action disables a channel; the console translates them.
var detectionDisableReasons = map[string]string{
	model.DetectionVerdictPassed:   "Disabled by a model replacement detection action",
	model.DetectionVerdictReplaced: "Model replacement detection found a replaced model",
	model.DetectionVerdictFailed:   "Model replacement detection requests failed after all retries",
}

// detectionTaskPayload is stored with the system task; its fields are those
// of the first fork release.
type detectionTaskPayload struct {
	Manual    bool `json:"manual,omitempty"`
	ChannelID int  `json:"channel_id,omitempty"`
	// ResumeAfterSeconds is what was left of the scheduled wait when a
	// single channel was retested, so the schedule continues afterwards.
	ResumeAfterSeconds int64 `json:"resume_after_seconds,omitempty"`
}

func (payload detectionTaskPayload) trigger() string {
	switch {
	case payload.ChannelID > 0:
		return model.DetectionTriggerRetest
	case payload.Manual:
		return model.DetectionTriggerManual
	default:
		return model.DetectionTriggerSchedule
	}
}

// detectionRunResult is stored as the system task result.
type detectionRunResult struct {
	StartedAt  int64  `json:"started_at"`
	FinishedAt int64  `json:"finished_at"`
	Trigger    string `json:"trigger"`
	Channels   int    `json:"channels"`
	Passed     int    `json:"passed"`
	Replaced   int    `json:"replaced"`
	Failed     int    `json:"failed"`
}

type modelReplacementHandler struct{}

func (modelReplacementHandler) Type() string { return model.SystemTaskTypeModelReplacementDetection }

func (modelReplacementHandler) Enabled() bool {
	return operation_setting.GetModelReplacementSetting().Enabled
}

func (modelReplacementHandler) Interval() time.Duration {
	return time.Duration(operation_setting.GetModelReplacementSetting().IntervalMinutes) * time.Minute
}

func (modelReplacementHandler) NewPayload() any { return detectionTaskPayload{} }

// IntervalAfter waits for the interval of the time of day the previous run
// finished in. After a single-channel retest it waits only what was left of
// the wait the retest interrupted.
func (modelReplacementHandler) IntervalAfter(previous *model.SystemTask, finishedAt time.Time) time.Duration {
	return detectionWaitAfter(operation_setting.GetModelReplacementSetting(), previous, finishedAt)
}

func detectionWaitAfter(setting operation_setting.ModelReplacementSetting, previous *model.SystemTask, finishedAt time.Time) time.Duration {
	var payload detectionTaskPayload
	if previous != nil && previous.DecodePayload(&payload) == nil && payload.ChannelID > 0 {
		return time.Duration(max(payload.ResumeAfterSeconds, 0)) * time.Second
	}
	return setting.IntervalAt(finishedAt)
}

// detectionNextRunAt is when the scheduler starts the next run.
func detectionNextRunAt(setting operation_setting.ModelReplacementSetting, latest *model.SystemTask, now time.Time) int64 {
	if latest == nil {
		return now.Unix()
	}
	wait := detectionWaitAfter(setting, latest, time.Unix(latest.UpdatedAt, 0))
	return max(now.Unix(), latest.UpdatedAt+int64(wait.Seconds()))
}

func (modelReplacementHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var payload detectionTaskPayload
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	result, err := runDetection(ctx, task.TaskID, payload, service.NewSystemTaskProgressReporter(task, runnerID))
	status := model.SystemTaskStatusSucceeded
	if err != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, result, err)
}

// detectionTarget is a channel and the monitored models it serves, with the
// probes collected for it. One worker handles a target at a time, so its
// requests are sequential.
type detectionTarget struct {
	channel    *model.Channel
	models     []operation_setting.MonitoredModel
	attempts   []model.ModelReplacementAttempt
	failed     bool
	startedAt  int64
	finishedAt int64
}

// monitoredModelsOf returns the monitored models a channel serves and is not
// excluded from.
func monitoredModelsOf(setting operation_setting.ModelReplacementSetting, channel *model.Channel) []operation_setting.MonitoredModel {
	if slices.Contains(unsupportedTestChannelTypes, channel.Type) {
		return nil
	}
	served := channel.GetModels()
	for i := range served {
		served[i] = strings.TrimSpace(served[i])
	}
	var models []operation_setting.MonitoredModel
	for _, item := range setting.Models {
		if slices.Contains(served, item.Model) && !slices.Contains(item.ExcludedChannelIDs, channel.Id) {
			models = append(models, item)
		}
	}
	return models
}

// detectionTargets selects the enabled channels in scope, or the one channel
// being retested whatever its status.
func detectionTargets(setting operation_setting.ModelReplacementSetting, channels []*model.Channel, channelID int) []*detectionTarget {
	var targets []*detectionTarget
	for _, channel := range channels {
		if channel == nil || (channelID > 0 && channel.Id != channelID) || (channelID == 0 && channel.Status != common.ChannelStatusEnabled) {
			continue
		}
		if models := monitoredModelsOf(setting, channel); len(models) > 0 {
			targets = append(targets, &detectionTarget{channel: channel, models: models})
		}
	}
	slices.SortFunc(targets, func(a, b *detectionTarget) int { return a.channel.Id - b.channel.Id })
	return targets
}

func runDetection(ctx context.Context, runID string, payload detectionTaskPayload, report func(processed, total int)) (detectionRunResult, error) {
	setting := operation_setting.GetModelReplacementSetting()
	result := detectionRunResult{StartedAt: common.GetTimestamp(), Trigger: payload.trigger()}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return result, err
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return result, err
	}
	targets := detectionTargets(setting, channels, payload.ChannelID)
	if payload.ChannelID > 0 && len(targets) == 0 {
		return result, fmt.Errorf("channel #%d serves no monitored model", payload.ChannelID)
	}

	timeout := time.Duration(setting.TimeoutSeconds) * time.Second
	total, processed := len(targets)*setting.Rounds, 0
	report(processed, total)
	for round := 1; round <= setting.Rounds; round++ {
		// A channel whose request kept failing sits out the remaining rounds.
		active := lo.Filter(targets, func(target *detectionTarget, _ int) bool { return !target.failed })
		processed += len(targets) - len(active)
		var progress sync.Mutex
		var workers sync.WaitGroup
		queue := make(chan *detectionTarget)
		for range min(setting.Concurrency, max(len(active), 1)) {
			workers.Go(func() {
				for target := range queue {
					probeTarget(ctx, target, round, testUserID, timeout)
					progress.Lock()
					processed++
					report(processed, total)
					progress.Unlock()
				}
			})
		}
		for _, target := range active {
			if ctx.Err() != nil {
				break
			}
			queue <- target
		}
		close(queue)
		workers.Wait()
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
	}

	cacheChanged := false
	for _, target := range targets {
		stats, verdict, matchPercent := summarizeDetection(target.models, target.attempts, setting.Threshold)
		record := &model.ModelReplacementRecord{
			RunID: runID, ChannelID: target.channel.Id, ChannelName: target.channel.Name,
			Verdict: verdict, MatchPercent: matchPercent, RunTrigger: result.Trigger,
			StartedAt: target.startedAt, FinishedAt: target.finishedAt,
		}
		applied, skipped, changed, applyErr := applyDetectionActions(target.channel.Id, verdict, detectionActionsFor(setting, verdict))
		cacheChanged = cacheChanged || changed
		record.ActionsSkipped = skipped
		if applyErr != nil {
			record.ErrorMessage = applyErr.Error()
			common.SysError(common.LogText("model replacement detection failed to update channel #%d: %s", target.channel.Id, applyErr.Error()))
		}
		if encoded, err := common.Marshal(stats); err == nil {
			record.ModelStats = string(encoded)
		}
		if encoded, err := common.Marshal(applied); err == nil {
			record.Actions = string(encoded)
		}
		if len(target.attempts) > 0 {
			record.AvgLatencyMs = lo.SumBy(target.attempts, func(attempt model.ModelReplacementAttempt) int64 { return attempt.LatencyMs }) / int64(len(target.attempts))
		}
		if err := model.CreateModelReplacementRecord(record, target.attempts); err != nil {
			common.SysError(common.LogText("model replacement detection failed to save channel #%d: %s", target.channel.Id, err.Error()))
		}
		result.Channels++
		switch verdict {
		case model.DetectionVerdictPassed:
			result.Passed++
		case model.DetectionVerdictReplaced:
			result.Replaced++
		default:
			result.Failed++
		}
	}
	if cacheChanged {
		model.InitChannelCache()
	}
	retention := time.Duration(setting.RetentionDays) * 24 * time.Hour
	if err := model.PurgeDetectionRecords(time.Now().Add(-retention).Unix()); err != nil {
		common.SysError(common.LogText("model replacement detection failed to delete old records: %s", err.Error()))
	}
	result.FinishedAt = common.GetTimestamp()
	return result, nil
}

func detectionActionsFor(setting operation_setting.ModelReplacementSetting, verdict string) []operation_setting.DetectionAction {
	switch verdict {
	case model.DetectionVerdictPassed:
		return setting.PassActions
	case model.DetectionVerdictReplaced:
		return setting.FailActions
	default:
		return setting.RequestFailureActions
	}
}

// probeTarget runs one round on a channel: each of its models once, in
// order. A model whose request still fails after its retries marks the
// channel failed and ends the round for it.
func probeTarget(ctx context.Context, target *detectionTarget, round int, testUserID int, timeout time.Duration) {
	for _, item := range target.models {
		if ctx.Err() != nil {
			return
		}
		attempt := probeModel(ctx, target.channel, item, item.RetryTimes, testUserID, timeout)
		attempt.Round = round
		if target.startedAt == 0 {
			target.startedAt = attempt.TestedAt
		}
		target.attempts = append(target.attempts, attempt)
		target.finishedAt = common.GetTimestamp()
		if !attempt.Success {
			target.failed = true
			return
		}
	}
}

// probeModel asks a model its question through a channel, retrying a failed
// request up to retries times, and judges the answer and the declared model.
func probeModel(ctx context.Context, channel *model.Channel, item operation_setting.MonitoredModel, retries int, testUserID int, timeout time.Duration) model.ModelReplacementAttempt {
	attempt := model.ModelReplacementAttempt{Model: item.Model, TestedAt: common.GetTimestamp()}
	for try := 0; try <= retries && ctx.Err() == nil; try++ {
		attempt.Tries = try + 1
		endpointType, isStream, request := detectionRequest(channel, item)
		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		startedAt := time.Now()
		result := probeChannel(probeCtx, channel, testUserID, item.Model, endpointType, isStream, request)
		attempt.LatencyMs = time.Since(startedAt).Milliseconds()
		timedOut := errors.Is(probeCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancel()
		if result.localErr != nil {
			attempt.ErrorMessage = detectionText(result.localErr.Error())
			if timedOut {
				attempt.ErrorMessage = fmt.Sprintf("request timed out after %s", timeout)
			}
			continue
		}
		answer, err := detectionAnswer(result.responseBody, isStream)
		if err != nil {
			attempt.ErrorMessage = err.Error()
			continue
		}
		modelOK, note := item.CheckResponseModel(result.declaredModel, result.mappedModel)
		attempt.Success, attempt.ErrorMessage = true, ""
		attempt.Answer = detectionText(answer)
		attempt.AnswerMatched = item.AnswerMatches(answer)
		attempt.ResponseModel = detectionText(result.declaredModel)
		attempt.ResponseModelNote = note
		attempt.Matched = attempt.AnswerMatched && modelOK
		break
	}
	return attempt
}

// detectionRequest asks the question through the endpoint the automatic
// channel test uses for this channel and model.
func detectionRequest(channel *model.Channel, item operation_setting.MonitoredModel) (string, bool, dto.Request) {
	isCodexChannel := shouldUseStreamForAutomaticChannelTest(channel)
	if isCodexChannel || strings.Contains(strings.ToLower(item.Model), "codex") {
		input, _ := common.Marshal([]map[string]string{{"role": "user", "content": item.Question}})
		return string(constant.EndpointTypeOpenAIResponse), isCodexChannel, &dto.OpenAIResponsesRequest{
			Model: item.Model, Input: input, Stream: lo.ToPtr(isCodexChannel),
		}
	}
	return string(constant.EndpointTypeOpenAI), false, &dto.GeneralOpenAIRequest{
		Model: item.Model, Stream: lo.ToPtr(false), Messages: []dto.Message{{Role: "user", Content: item.Question}},
	}
}

// detectionAnswer reads the reply text of a chat completion or a Responses
// result, or of either's event stream.
func detectionAnswer(body []byte, isStream bool) (string, error) {
	var answer strings.Builder
	switch {
	case isStream:
		for line := range bytes.SplitSeq(body, []byte{'\n'}) {
			data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
			data = bytes.TrimSpace(data)
			if !ok || len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
				continue
			}
			event := gjson.ParseBytes(data)
			switch event.Get("type").String() {
			case "response.output_text.delta":
				answer.WriteString(event.Get("delta").String())
			case "response.completed":
				if answer.Len() == 0 {
					answer.WriteString(responsesOutputText(event.Get("response.output")))
				}
			case "":
				answer.WriteString(event.Get("choices.0.delta.content").String())
			}
		}
	case gjson.GetBytes(body, "output").IsArray():
		answer.WriteString(responsesOutputText(gjson.GetBytes(body, "output")))
	default:
		content := gjson.GetBytes(body, "choices.0.message.content")
		if !content.IsArray() {
			answer.WriteString(content.String())
			break
		}
		for _, part := range content.Array() {
			answer.WriteString(part.Get("text").String())
		}
	}
	text := strings.TrimSpace(answer.String())
	if text == "" {
		return "", errors.New("upstream returned an empty answer")
	}
	return text, nil
}

func responsesOutputText(output gjson.Result) string {
	var text strings.Builder
	for _, item := range output.Array() {
		for _, part := range item.Get("content").Array() {
			if part.Get("type").String() == "output_text" {
				text.WriteString(part.Get("text").String())
			}
		}
	}
	return text.String()
}

func detectionText(text string) string {
	if utf8.RuneCountInString(text) <= detectionStoredTextLimit {
		return text
	}
	return string([]rune(text)[:detectionStoredTextLimit])
}

// summarizeDetection computes each model's result and the channel verdict.
// A model counts as replaced when its share of mismatched rounds is above the
// threshold; a request that kept failing makes the channel failed, which
// outweighs replaced. matchPercent is the lowest model match rate.
func summarizeDetection(models []operation_setting.MonitoredModel, attempts []model.ModelReplacementAttempt, threshold float64) ([]model.DetectionModelStat, string, float64) {
	stats := make([]model.DetectionModelStat, 0, len(models))
	verdict, matchPercent := model.DetectionVerdictPassed, 100.0
	for _, item := range models {
		stat := model.DetectionModelStat{Model: item.Model}
		for _, attempt := range attempts {
			if attempt.Model != item.Model {
				continue
			}
			stat.Total++
			switch {
			case !attempt.Success:
				stat.Failed++
			case attempt.Matched:
				stat.Matched++
			default:
				stat.Mismatched++
			}
			if attempt.ResponseModelNote == operation_setting.ResponseModelMismatch {
				stat.ResponseModelMismatch = true
			}
		}
		if stat.Total > 0 {
			stat.MatchPercent = float64(stat.Matched) * 100 / float64(stat.Total)
			stat.MismatchPercent = float64(stat.Mismatched) * 100 / float64(stat.Total)
			matchPercent = min(matchPercent, stat.MatchPercent)
		}
		switch {
		case stat.Failed > 0:
			stat.Verdict, verdict = model.DetectionVerdictFailed, model.DetectionVerdictFailed
		case stat.Total == 0:
			// Not reached: an earlier model of the channel failed.
		case stat.MismatchPercent > threshold:
			stat.Verdict = model.DetectionVerdictReplaced
			if verdict != model.DetectionVerdictFailed {
				verdict = model.DetectionVerdictReplaced
			}
		default:
			stat.Verdict = model.DetectionVerdictPassed
		}
		stats = append(stats, stat)
	}
	if len(attempts) == 0 {
		matchPercent = 0
	}
	return stats, verdict, matchPercent
}

// rewriteGroups removes the named entries from a comma-separated group list
// and appends add unless present. Other entries stay as written, in place.
func rewriteGroups(group string, add string, remove string) string {
	var entries []string
	hasAdd := false
	if group != "" {
		for entry := range strings.SplitSeq(group, ",") {
			name := strings.TrimSpace(entry)
			switch {
			case name != "" && name == add:
				hasAdd = true
			case name != "" && name == remove:
				continue
			}
			entries = append(entries, entry)
		}
	}
	if add != "" && !hasAdd {
		entries = append(entries, add)
	}
	return strings.Join(entries, ",")
}

// applyDetectionActions runs the actions on a channel in order and writes
// the outcome once. The channel is read again so edits made during the run
// are kept; a channel that is no longer enabled is left alone and reported
// as skipped. changed reports a write that needs a channel cache refresh.
func applyDetectionActions(channelID int, verdict string, actions []operation_setting.DetectionAction) ([]model.DetectionAppliedAction, bool, bool, error) {
	applied := make([]model.DetectionAppliedAction, 0, len(actions))
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return applied, false, false, err
	}
	if channel.Status != common.ChannelStatusEnabled {
		return applied, len(actions) > 0, false, nil
	}

	const limit = operation_setting.DetectionRoutingLimit
	group, priority, weight, tag := channel.Group, channel.GetPriority(), int64(channel.GetWeight()), channel.GetTag()
	disableStatus := 0
	for _, action := range actions {
		step := model.DetectionAppliedAction{Type: action.Type, Value: action.Value, Amount: action.Amount}
		before := fmt.Sprint(group, priority, weight, tag)
		switch action.Type {
		case operation_setting.DetectionActionAddGroup:
			group = rewriteGroups(group, action.Value, "")
		case operation_setting.DetectionActionRemoveGroup:
			group = rewriteGroups(group, "", action.Value)
		case operation_setting.DetectionActionIncreasePriority:
			priority = min(priority+action.Amount, limit)
		case operation_setting.DetectionActionDecreasePriority:
			priority = max(priority-action.Amount, -limit)
		case operation_setting.DetectionActionSetPriority:
			priority = action.Amount
		case operation_setting.DetectionActionIncreaseWeight:
			weight = min(weight+action.Amount, limit)
		case operation_setting.DetectionActionDecreaseWeight:
			weight = max(weight-action.Amount, 0)
		case operation_setting.DetectionActionSetWeight:
			weight = action.Amount
		case operation_setting.DetectionActionSetTag:
			tag = action.Value
		case operation_setting.DetectionActionRemoveTag:
			if action.Value == "" || action.Value == tag {
				tag = ""
			}
		case operation_setting.DetectionActionAutoDisable:
			disableStatus = common.ChannelStatusAutoDisabled
		case operation_setting.DetectionActionManualDisable:
			disableStatus = common.ChannelStatusManuallyDisabled
		}
		step.Changed = fmt.Sprint(group, priority, weight, tag) != before || action.Type == operation_setting.DetectionActionAutoDisable || action.Type == operation_setting.DetectionActionManualDisable
		applied = append(applied, step)
	}

	changed := false
	var errs []error
	if group != channel.Group || priority != channel.GetPriority() || weight != int64(channel.GetWeight()) || tag != channel.GetTag() {
		if len(group) > 64 {
			errs = append(errs, fmt.Errorf("group list %q exceeds 64 characters", group))
			group = channel.Group
		}
		var tagValue *string
		if tag != "" {
			tagValue = &tag
		}
		if err := model.UpdateChannelRouting(channel.Id, group, priority, uint(weight), tagValue); err != nil {
			errs = append(errs, err)
		} else {
			changed = true
		}
	}
	reason := detectionDisableReasons[verdict]
	switch disableStatus {
	case common.ChannelStatusAutoDisabled:
		service.DisableChannel(*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", true), reason)
		changed = true
	case common.ChannelStatusManuallyDisabled:
		if model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, reason) {
			closeActiveChannelWebSockets([]int{channel.Id})
			changed = true
		}
	}
	return applied, false, changed, errors.Join(errs...)
}
