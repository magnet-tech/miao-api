package controller

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	detectionSettingsBodyLimit = 1 << 20
	maxDetectionRunsListed     = 100
)

// detectionChannel is what the detection pages show of a channel; never its
// credentials.
type detectionChannel struct {
	ID       int      `json:"id"`
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Groups   []string `json:"groups"`
	Priority int64    `json:"priority"`
	Weight   int      `json:"weight"`
	Tag      string   `json:"tag"`
}

func newDetectionChannel(channel *model.Channel) detectionChannel {
	status := "enabled"
	switch channel.Status {
	case common.ChannelStatusAutoDisabled:
		status = "auto_disabled"
	case common.ChannelStatusManuallyDisabled:
		status = "manual_disabled"
	}
	groups := []string{}
	for _, group := range channel.GetGroups() {
		if group = strings.TrimSpace(group); group != "" && !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	return detectionChannel{ID: channel.Id, Name: channel.Name, Status: status, Groups: groups,
		Priority: channel.GetPriority(), Weight: channel.GetWeight(), Tag: channel.GetTag()}
}

type detectionRunSummary struct {
	ID         string         `json:"id"`
	Trigger    string         `json:"trigger"`
	ChannelID  *int           `json:"channel_id"`
	StartedAt  int64          `json:"started_at"`
	FinishedAt int64          `json:"finished_at"`
	DurationMs int64          `json:"duration_ms"`
	Counts     map[string]int `json:"counts"`
}

type detectionCell struct {
	ChannelID             int     `json:"channel_id,omitempty"`
	Model                 string  `json:"model"`
	Verdict               string  `json:"verdict"`
	MatchPercent          float64 `json:"match_percent"`
	ResponseModelMismatch bool    `json:"response_model_mismatch"`
}

type detectionChannelResult struct {
	ChannelID      int                            `json:"channel_id"`
	Verdict        string                         `json:"verdict"`
	Actions        []model.DetectionAppliedAction `json:"actions"`
	SkippedActions bool                           `json:"skipped_actions"`
}

type detectionRunView struct {
	detectionRunSummary
	Cells          []detectionCell          `json:"cells"`
	ChannelResults []detectionChannelResult `json:"channel_results"`
}

func detectionCells(record *model.ModelReplacementRecord, withChannel bool) []detectionCell {
	cells := []detectionCell{}
	for _, stat := range record.DecodeModelStats() {
		cell := detectionCell{Model: stat.Model, Verdict: stat.Verdict, MatchPercent: stat.MatchPercent, ResponseModelMismatch: stat.ResponseModelMismatch}
		if cell.Verdict == "" {
			cell.Verdict = "not_tested"
		}
		if withChannel {
			cell.ChannelID = record.ChannelID
		}
		cells = append(cells, cell)
	}
	return cells
}

// detectionRunViews groups records into runs, in the order of runIDs.
// Rows of the first release carry no trigger; it comes from their task.
func detectionRunViews(runIDs []string, records []*model.ModelReplacementRecord) []detectionRunView {
	byRun := make(map[string]*detectionRunView, len(runIDs))
	for _, record := range records {
		run := byRun[record.RunID]
		if run == nil {
			run = &detectionRunView{
				detectionRunSummary: detectionRunSummary{ID: record.RunID, Trigger: record.RunTrigger, StartedAt: record.StartedAt,
					Counts: map[string]int{model.DetectionVerdictPassed: 0, model.DetectionVerdictReplaced: 0, model.DetectionVerdictFailed: 0}},
				Cells: []detectionCell{}, ChannelResults: []detectionChannelResult{},
			}
			byRun[record.RunID] = run
		}
		run.StartedAt = min(run.StartedAt, record.StartedAt)
		run.FinishedAt = max(run.FinishedAt, record.FinishedAt)
		run.Counts[record.Verdict]++
		run.Cells = append(run.Cells, detectionCells(record, true)...)
		run.ChannelResults = append(run.ChannelResults, detectionChannelResult{
			ChannelID: record.ChannelID, Verdict: record.Verdict, Actions: record.DecodeActions(), SkippedActions: record.ActionsSkipped,
		})
	}
	views := make([]detectionRunView, 0, len(runIDs))
	for _, runID := range runIDs {
		run := byRun[runID]
		if run == nil {
			continue
		}
		if run.Trigger == "" {
			run.Trigger = model.DetectionTriggerSchedule
			if task, err := model.GetSystemTaskByTaskID(runID); err == nil {
				var payload detectionTaskPayload
				if task.DecodePayload(&payload) == nil {
					run.Trigger = payload.trigger()
				}
			}
		}
		if run.Trigger == model.DetectionTriggerRetest && len(run.ChannelResults) > 0 {
			run.ChannelID = &run.ChannelResults[0].ChannelID
		}
		run.DurationMs = max(run.FinishedAt-run.StartedAt, 0) * 1000
		views = append(views, *run)
	}
	return views
}

func GetDetectionSettings(c *gin.Context) {
	common.ApiSuccess(c, gin.H{"settings": operation_setting.GetModelReplacementSetting(), "server_time_zone": serverTimeZone()})
}

func UpdateDetectionSettings(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, detectionSettingsBodyLimit))
	if err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	setting, err := operation_setting.ParseModelReplacementSetting(string(body))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	value, err := common.Marshal(setting)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOption(operation_setting.ModelReplacementSettingOptionKey, string(value)); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "model_replacement.settings_update", map[string]any{"enabled": setting.Enabled, "models": len(setting.Models)})
	common.ApiSuccess(c, gin.H{"settings": setting, "server_time_zone": serverTimeZone()})
}

// GetDetectionCandidates lists the models that can be monitored, each with
// the channels serving it: the models of enabled channels and those already
// monitored.
func GetDetectionCandidates(c *gin.Context) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	slices.SortFunc(channels, func(a, b *model.Channel) int { return a.Id - b.Id })
	served := map[string][]detectionChannel{}
	for _, item := range operation_setting.GetModelReplacementSetting().Models {
		served[item.Model] = []detectionChannel{}
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled || slices.Contains(unsupportedTestChannelTypes, channel.Type) {
			continue
		}
		for _, name := range channel.GetModels() {
			if name = strings.TrimSpace(name); name != "" && served[name] == nil {
				served[name] = []detectionChannel{}
			}
		}
	}
	for _, channel := range channels {
		if slices.Contains(unsupportedTestChannelTypes, channel.Type) {
			continue
		}
		view := newDetectionChannel(channel)
		for _, name := range channel.GetModels() {
			name = strings.TrimSpace(name)
			if list, ok := served[name]; ok && !slices.ContainsFunc(list, func(item detectionChannel) bool { return item.ID == channel.Id }) {
				served[name] = append(list, view)
			}
		}
	}
	type candidate struct {
		Model    string             `json:"model"`
		Channels []detectionChannel `json:"channels"`
	}
	candidates := make([]candidate, 0, len(served))
	for name, list := range served {
		candidates = append(candidates, candidate{Model: name, Channels: list})
	}
	slices.SortFunc(candidates, func(a, b candidate) int { return strings.Compare(a.Model, b.Model) })
	common.ApiSuccess(c, candidates)
}

type detectionLatest struct {
	RunID          string                         `json:"run_id"`
	StartedAt      int64                          `json:"started_at"`
	FinishedAt     int64                          `json:"finished_at"`
	Trigger        string                         `json:"trigger"`
	Verdict        string                         `json:"verdict"`
	AvgLatencyMs   int64                          `json:"avg_latency_ms"`
	Cells          []detectionCell                `json:"cells"`
	Actions        []model.DetectionAppliedAction `json:"actions"`
	SkippedActions bool                           `json:"skipped_actions"`
}

type detectionChannelRow struct {
	detectionChannel
	InScope         bool             `json:"in_scope"`
	MonitoredModels []string         `json:"monitored_models"`
	Latest          *detectionLatest `json:"latest"`
}

// detectionChannelRows covers the channels in scope and those detected
// before, each with its latest result.
func detectionChannelRows(setting operation_setting.ModelReplacementSetting) ([]detectionChannelRow, error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}
	records, err := model.GetLatestDetectionRecords()
	if err != nil {
		return nil, err
	}
	latest := make(map[int]*model.ModelReplacementRecord, len(records))
	for _, record := range records {
		latest[record.ChannelID] = record
	}
	rows := []detectionChannelRow{}
	for _, channel := range channels {
		row := detectionChannelRow{detectionChannel: newDetectionChannel(channel), MonitoredModels: []string{}}
		for _, item := range monitoredModelsOf(setting, channel) {
			row.MonitoredModels = append(row.MonitoredModels, item.Model)
		}
		row.InScope = len(row.MonitoredModels) > 0
		if record := latest[channel.Id]; record != nil {
			trigger := record.RunTrigger
			if trigger == "" {
				trigger = model.DetectionTriggerSchedule
			}
			row.Latest = &detectionLatest{
				RunID: record.RunID, StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, Trigger: trigger,
				Verdict: record.Verdict, AvgLatencyMs: record.AvgLatencyMs, Cells: detectionCells(record, false),
				Actions: record.DecodeActions(), SkippedActions: record.ActionsSkipped,
			}
		}
		if row.InScope || row.Latest != nil {
			rows = append(rows, row)
		}
	}
	slices.SortFunc(rows, func(a, b detectionChannelRow) int { return a.ID - b.ID })
	return rows, nil
}

func GetDetectionChannels(c *gin.Context) {
	rows, err := detectionChannelRows(operation_setting.GetModelReplacementSetting())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

type detectionActiveRun struct {
	ID        string `json:"id"`
	Trigger   string `json:"trigger"`
	ChannelID *int   `json:"channel_id"`
	StartedAt int64  `json:"started_at"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
}

func newDetectionActiveRun(task *model.SystemTask) *detectionActiveRun {
	var payload detectionTaskPayload
	_ = task.DecodePayload(&payload)
	var progress service.SystemTaskProgress
	_ = task.DecodeState(&progress)
	run := &detectionActiveRun{ID: task.TaskID, Trigger: payload.trigger(), StartedAt: task.CreatedAt, Done: progress.Processed, Total: progress.Total}
	if payload.ChannelID > 0 {
		run.ChannelID = &payload.ChannelID
	}
	return run
}

// GetDetectionOverview reports the schedule, the last and the running run,
// and how the channels in scope stand.
func GetDetectionOverview(c *gin.Context) {
	setting := operation_setting.GetModelReplacementSetting()
	taskType := model.SystemTaskTypeModelReplacementDetection
	active, err := model.GetActiveSystemTask(taskType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	latestTask, err := model.GetLatestSystemTask(taskType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows, err := detectionChannelRows(setting)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	counts := map[string]int{"healthy": 0, "suspected": 0, "failed": 0, "not_tested": 0, "in_scope": 0}
	for _, row := range rows {
		if !row.InScope {
			continue
		}
		counts["in_scope"]++
		switch {
		case row.Latest == nil:
			counts["not_tested"]++
		case row.Latest.Verdict == model.DetectionVerdictReplaced:
			counts["suspected"]++
		case row.Latest.Verdict == model.DetectionVerdictFailed:
			counts["failed"]++
		case row.Status == "enabled":
			counts["healthy"]++
		}
	}
	models := make([]string, 0, len(setting.Models))
	for _, item := range setting.Models {
		models = append(models, item.Model)
	}
	now := time.Now()
	data := gin.H{
		"enabled": setting.Enabled, "interval_minutes": int(setting.IntervalAt(now) / time.Minute),
		"server_time_zone": serverTimeZone(), "rounds": setting.Rounds, "models": models, "counts": counts,
		"next_run_at": nil, "last_run": nil, "active_run": nil,
	}
	if runs, _, err := model.ListDetectionRuns(1); err == nil && len(runs) > 0 {
		records, err := model.GetDetectionRecordsOfRuns([]string{runs[0].RunID})
		if err == nil {
			if views := detectionRunViews([]string{runs[0].RunID}, records); len(views) > 0 {
				data["last_run"] = views[0].detectionRunSummary
			}
		}
	}
	if active != nil {
		data["active_run"] = newDetectionActiveRun(active)
	} else if setting.Enabled {
		data["next_run_at"] = detectionNextRunAt(setting, latestTask, now)
	}
	common.ApiSuccess(c, data)
}

// ListDetectionRuns returns the newest runs with their cells, for the
// overview tables.
func ListDetectionRuns(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if err != nil || limit < 1 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	runs, total, err := model.ListDetectionRuns(min(limit, maxDetectionRunsListed))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		runIDs = append(runIDs, run.RunID)
	}
	records, err := model.GetDetectionRecordsOfRuns(runIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"runs": detectionRunViews(runIDs, records), "total": total})
}

func GetDetectionRun(c *gin.Context) {
	runID := c.Param("id")
	records, err := model.GetDetectionRecordsOfRuns([]string{runID})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views := detectionRunViews([]string{runID}, records)
	if len(views) == 0 {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Detection record not found"))
		return
	}
	common.ApiSuccess(c, views[0])
}

type detectionAttemptView struct {
	Round             int    `json:"round"`
	Answer            string `json:"answer"`
	AnswerMatched     bool   `json:"answer_matched"`
	ResponseModel     string `json:"response_model"`
	ResponseModelOK   bool   `json:"response_model_ok"`
	ResponseModelNote string `json:"response_model_note"`
	Matched           bool   `json:"matched"`
	LatencyMs         int64  `json:"latency_ms"`
	Tries             int    `json:"tries"`
	Error             string `json:"error"`
}

func newDetectionAttemptView(attempt model.ModelReplacementAttempt) detectionAttemptView {
	view := detectionAttemptView{
		Round: attempt.Round, Answer: attempt.Answer, AnswerMatched: attempt.AnswerMatched, ResponseModel: attempt.ResponseModel,
		ResponseModelNote: attempt.ResponseModelNote, Matched: attempt.Matched, LatencyMs: attempt.LatencyMs, Tries: attempt.Tries, Error: attempt.ErrorMessage,
	}
	if view.ResponseModelNote == "" {
		// A first-release probe: a declared model it rejected was stored as
		// its error, with the answer itself counted as wrong.
		view.ResponseModelNote = operation_setting.ResponseModelNotChecked
		view.AnswerMatched = attempt.Matched
		if attempt.Success && attempt.ErrorMessage == legacyResponseModelMismatch {
			view.ResponseModelNote, view.AnswerMatched, view.Error = operation_setting.ResponseModelMismatch, true, ""
		}
	}
	view.ResponseModelOK = attempt.Success && view.ResponseModelNote != operation_setting.ResponseModelMismatch
	return view
}

// GetDetectionCell returns how one model of one channel fared in a run,
// round by round, with the actions its channel got.
func GetDetectionCell(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Query("channel_id"))
	modelName := c.Query("model")
	if err != nil || channelID <= 0 || modelName == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	runID := c.Param("id")
	record, err := model.GetDetectionRecord(runID, channelID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Detection record not found"))
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	attempts, err := model.GetDetectionAttempts(record.ID, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	runs := detectionRunViews([]string{runID}, []*model.ModelReplacementRecord{record})
	channel := detectionChannel{ID: record.ChannelID, Name: record.ChannelName, Groups: []string{}, Status: "enabled"}
	if current, err := model.GetChannelById(channelID, false); err == nil {
		channel = newDetectionChannel(current)
	}
	data := gin.H{
		"run": runs[0].detectionRunSummary, "channel": channel, "model": modelName,
		"verdict": "not_tested", "match_percent": 0, "channel_verdict": record.Verdict,
		"attempts": []detectionAttemptView{}, "actions": record.DecodeActions(), "skipped_actions": record.ActionsSkipped,
		"question": "", "expected_answer": "", "match_mode": operation_setting.MatchModeExact, "response_model_check": operation_setting.ResponseModelCheckLoose,
	}
	for _, cell := range detectionCells(record, false) {
		if cell.Model == modelName {
			data["verdict"], data["match_percent"] = cell.Verdict, cell.MatchPercent
		}
	}
	views := make([]detectionAttemptView, 0, len(attempts))
	for _, attempt := range attempts {
		views = append(views, newDetectionAttemptView(attempt))
	}
	data["attempts"] = views
	// The question and checks are the current ones; they may have changed
	// since the run.
	for _, item := range operation_setting.GetModelReplacementSetting().Models {
		if item.Model == modelName {
			data["question"], data["expected_answer"] = item.Question, item.ExpectedAnswer
			data["match_mode"], data["response_model_check"] = item.MatchMode, item.ResponseModelCheck
		}
	}
	common.ApiSuccess(c, data)
}

func enqueueDetection(c *gin.Context, payload detectionTaskPayload) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeModelReplacementDetection, payload)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("A model replacement detection run is already running or queued"))
		return
	}
	recordManageAudit(c, "model_replacement.run", map[string]any{"trigger": payload.trigger(), "channel_id": payload.ChannelID})
	common.ApiSuccess(c, newDetectionActiveRun(task))
}

// StartDetectionRun queues a run of every channel in scope; it works while
// scheduled detection is off.
func StartDetectionRun(c *gin.Context) {
	setting := operation_setting.GetModelReplacementSetting()
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(detectionTargets(setting, channels, 0)) == 0 {
		common.ApiErrorT(c, "No channels are in detection scope yet.")
		return
	}
	enqueueDetection(c, detectionTaskPayload{Manual: true})
}

// RetestDetectionChannel queues a run of one channel, enabled or not. The
// wait left before the next scheduled run is stored with it, so the schedule
// resumes with the same time left.
func RetestDetectionChannel(c *gin.Context) {
	setting := operation_setting.GetModelReplacementSetting()
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Channel does not exist"))
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(monitoredModelsOf(setting, channel)) == 0 {
		common.ApiErrorT(c, "This channel serves no monitored model.")
		return
	}
	latest, err := model.GetLatestSystemTask(model.SystemTaskTypeModelReplacementDetection)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	enqueueDetection(c, detectionTaskPayload{Manual: true, ChannelID: channelID,
		ResumeAfterSeconds: max(detectionNextRunAt(setting, latest, now)-now.Unix(), 0)})
}

type detectionTrialRequest struct {
	ChannelID          int    `json:"channel_id"`
	Model              string `json:"model"`
	Question           string `json:"question"`
	ExpectedAnswer     string `json:"expected_answer"`
	MatchMode          string `json:"match_mode"`
	ResponseModelCheck string `json:"response_model_check"`
}

// TrialDetection asks a channel a model's question once, as a run would,
// without storing a result or acting on the channel.
func TrialDetection(c *gin.Context) {
	var request detectionTrialRequest
	if err := common.DecodeJson(io.LimitReader(c.Request.Body, detectionSettingsBodyLimit), &request); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	item := operation_setting.MonitoredModel{
		Model: request.Model, Question: request.Question, ExpectedAnswer: request.ExpectedAnswer,
		MatchMode: request.MatchMode, ResponseModelCheck: request.ResponseModelCheck,
	}
	if err := item.Normalize(); err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.GetChannelById(request.ChannelID, true)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Channel does not exist"))
		return
	}
	served := channel.GetModels()
	for i := range served {
		served[i] = strings.TrimSpace(served[i])
	}
	if !slices.Contains(served, item.Model) || slices.Contains(unsupportedTestChannelTypes, channel.Type) {
		common.ApiErrorT(c, "This channel does not serve the model.")
		return
	}
	testUserID, err := resolveChannelTestUserID(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setting := operation_setting.GetModelReplacementSetting()
	attempt := probeModel(c.Request.Context(), channel, item, 0, testUserID, time.Duration(setting.TimeoutSeconds)*time.Second)
	view := newDetectionAttemptView(attempt)
	verdict := model.DetectionVerdictPassed
	switch {
	case !attempt.Success:
		verdict = model.DetectionVerdictFailed
	case !attempt.Matched:
		verdict = model.DetectionVerdictReplaced
	}
	common.ApiSuccess(c, gin.H{
		"channel": newDetectionChannel(channel), "model": item.Model, "answer": view.Answer, "answer_matched": view.AnswerMatched,
		"response_model": view.ResponseModel, "response_model_ok": view.ResponseModelOK, "response_model_note": view.ResponseModelNote,
		"latency_ms": view.LatencyMs, "error": view.Error, "verdict": verdict,
	})
}
