package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectionSettingsKeepFirstReleaseValues(t *testing.T) {
	// A value as the first fork release stored it: no retention and no
	// response model check yet.
	stored := `{"enabled":true,"interval_minutes":15,"interval_windows":[{"start":"01:00","end":"07:00","interval_minutes":30}],
		"rounds":3,"concurrency":2,"timeout_seconds":60,"threshold":10,
		"pass_actions":[{"type":"add_group","value":" vip ","amount":0}],"fail_actions":[{"type":"decrease_priority","value":"","amount":10}],
		"request_failure_actions":[{"type":"auto_disable","value":"","amount":0}],
		"models":[{"model":"gpt-4o-mini","question":"6*7?","expected_answer":"42","match_mode":"exact","retry_times":2,"excluded_channel_ids":[9,3,9]}]}`
	setting, err := operation_setting.ParseModelReplacementSetting(stored)
	require.NoError(t, err)
	assert.Equal(t, 7, setting.RetentionDays)
	assert.Equal(t, operation_setting.ResponseModelCheckLoose, setting.Models[0].ResponseModelCheck)
	assert.Equal(t, []int{3, 9}, setting.Models[0].ExcludedChannelIDs)
	assert.Equal(t, "vip", setting.PassActions[0].Value)
	assert.Equal(t, int64(10), setting.FailActions[0].Amount)
	assert.Equal(t, 30*time.Minute, setting.IntervalAt(time.Date(2026, 11, 17, 3, 0, 0, 0, time.Local)))
	assert.Equal(t, 15*time.Minute, setting.IntervalAt(time.Date(2026, 11, 17, 7, 0, 0, 0, time.Local)))

	for name, change := range map[string]func(*operation_setting.ModelReplacementSetting){
		"overlapping windows": func(s *operation_setting.ModelReplacementSetting) {
			s.IntervalWindows = append(s.IntervalWindows, operation_setting.IntervalWindow{Start: "23:00", End: "02:00", IntervalMinutes: 5})
		},
		"retention over a year":  func(s *operation_setting.ModelReplacementSetting) { s.RetentionDays = 366 },
		"unknown response check": func(s *operation_setting.ModelReplacementSetting) { s.Models[0].ResponseModelCheck = "fuzzy" },
		"invalid regex": func(s *operation_setting.ModelReplacementSetting) {
			s.Models[0].MatchMode, s.Models[0].ExpectedAnswer = operation_setting.MatchModeRegex, "("
		},
		"duplicate model": func(s *operation_setting.ModelReplacementSetting) { s.Models = append(s.Models, s.Models[0]) },
		"weight below zero": func(s *operation_setting.ModelReplacementSetting) {
			s.FailActions = []operation_setting.DetectionAction{{Type: operation_setting.DetectionActionSetWeight, Amount: -1}}
		},
		"group with comma": func(s *operation_setting.ModelReplacementSetting) {
			s.PassActions = []operation_setting.DetectionAction{{Type: operation_setting.DetectionActionAddGroup, Value: "a,b"}}
		},
	} {
		changed := setting
		changed.Models = append([]operation_setting.MonitoredModel{}, setting.Models...)
		changed.IntervalWindows = append([]operation_setting.IntervalWindow{}, setting.IntervalWindows...)
		change(&changed)
		encoded, err := common.Marshal(changed)
		require.NoError(t, err)
		_, err = operation_setting.ParseModelReplacementSetting(string(encoded))
		assert.Error(t, err, name)
	}
}

func TestResponseModelCheck(t *testing.T) {
	cases := []struct {
		check, declared, mapped string
		ok                      bool
		note                    string
	}{
		{"loose", "", "", true, "not_checked"},
		{"off", "gpt-3.5-turbo", "", true, "not_checked"},
		{"exact", "gpt-4o", "", true, "exact"},
		{"exact", "GPT-4o", "", false, "mismatch"},
		{"loose", "GPT-4o-2024-08-06", "", true, "loose_accepted"},
		{"loose", "openai/gpt-4o-20240806", "", true, "loose_accepted"},
		{"loose", "gpt-4o-mini", "", false, "mismatch"},
		{"exact", "azure-4o", "azure-4o", true, "exact"},
		{"loose", "Azure-4o-2025-01-01", "azure-4o", true, "loose_accepted"},
	}
	for _, tc := range cases {
		item := operation_setting.MonitoredModel{Model: "gpt-4o", ResponseModelCheck: tc.check}
		ok, note := item.CheckResponseModel(tc.declared, tc.mapped)
		assert.Equal(t, [2]any{tc.ok, tc.note}, [2]any{ok, note}, "%s %q", tc.check, tc.declared)
	}
}

// detectionUpstream answers chat completions by the channel key: good
// answers right, dated declares a dated model, swapped declares another
// model, wrong answers wrongly and down fails.
func detectionUpstream(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		modelName, answer := "gpt-4o-mini", "42"
		switch key {
		case "sk-down":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":{"message":"bad gateway","type":"upstream"}}`))
			return
		case "sk-dated":
			modelName = "gpt-4o-mini-2024-07-18"
		case "sk-swapped":
			modelName = "gpt-3.5-turbo"
		case "sk-wrong":
			answer = "forty-two"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"x","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`, modelName, answer)
	}))
	t.Cleanup(server.Close)
	return server
}

func setupDetectionTest(t *testing.T) map[string]*model.Channel {
	t.Helper()
	setupAdminUserTest(t)
	initDialectColumns(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.SystemTask{}, &model.ModelReplacementRecord{}, &model.ModelReplacementAttempt{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	previous, previousSelfUse := operation_setting.ModelReplacementSettingJSON(), operation_setting.SelfUseModeEnabled
	// Probes are priced like channel tests; self-use mode needs no price.
	operation_setting.SelfUseModeEnabled = true
	t.Cleanup(func() {
		operation_setting.LoadModelReplacementSetting(previous)
		operation_setting.SelfUseModeEnabled = previousSelfUse
	})

	server := detectionUpstream(t)
	channels := map[string]*model.Channel{}
	for _, key := range []string{"good", "dated", "swapped", "wrong", "down"} {
		baseURL, priority, weight := server.URL, int64(0), uint(10)
		channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: key, Key: "sk-" + key, BaseURL: &baseURL, Status: common.ChannelStatusEnabled,
			Models: "gpt-4o-mini", Group: "default", Priority: &priority, Weight: &weight}
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
		channels[key] = channel
	}
	setting := operation_setting.DefaultModelReplacementSetting()
	setting.Rounds, setting.Concurrency, setting.TimeoutSeconds = 2, 3, 10
	setting.Models = []operation_setting.MonitoredModel{{Model: "gpt-4o-mini", Question: "6*7? Reply with the number.", ExpectedAnswer: "42",
		MatchMode: operation_setting.MatchModeExact, ResponseModelCheck: operation_setting.ResponseModelCheckLoose, RetryTimes: 1}}
	setting.PassActions = []operation_setting.DetectionAction{{Type: operation_setting.DetectionActionAddGroup, Value: "vip"}}
	setting.FailActions = []operation_setting.DetectionAction{{Type: operation_setting.DetectionActionDecreasePriority, Amount: 10}, {Type: operation_setting.DetectionActionSetTag, Value: "suspect"}}
	setting.RequestFailureActions = []operation_setting.DetectionAction{{Type: operation_setting.DetectionActionManualDisable}}
	encoded, err := common.Marshal(setting)
	require.NoError(t, err)
	operation_setting.LoadModelReplacementSetting(string(encoded))
	return channels
}

func detectionAPI(t *testing.T, handler gin.HandlerFunc, path string, params gin.Params) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	c.Params = params
	handler(c)
	var body struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
	require.True(t, body.Success, body.Message)
	return body.Data
}

func TestDetectionRunJudgesChannelsAndActs(t *testing.T) {
	channels := setupDetectionTest(t)

	result, err := runDetection(context.Background(), "run-1", detectionTaskPayload{}, func(int, int) {})
	require.NoError(t, err)
	assert.Equal(t, detectionRunResult{StartedAt: result.StartedAt, FinishedAt: result.FinishedAt, Trigger: "schedule", Channels: 5, Passed: 2, Replaced: 2, Failed: 1}, result)

	verdicts := map[string]string{}
	records, err := model.GetDetectionRecordsOfRuns([]string{"run-1"})
	require.NoError(t, err)
	for _, record := range records {
		verdicts[record.ChannelName] = record.Verdict
	}
	assert.Equal(t, map[string]string{"good": "passed", "dated": "passed", "swapped": "replaced", "wrong": "replaced", "down": "failed"}, verdicts)

	reload := func(name string) *model.Channel {
		channel, err := model.GetChannelById(channels[name].Id, true)
		require.NoError(t, err)
		return channel
	}
	assert.Equal(t, "default,vip", reload("good").Group, "passed channels join vip")
	var vipAbilities int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where(map[string]any{"channel_id": channels["good"].Id, "group": "vip"}).Count(&vipAbilities).Error)
	assert.Equal(t, int64(1), vipAbilities, "abilities follow the new group")
	swapped := reload("swapped")
	assert.Equal(t, int64(-10), swapped.GetPriority())
	assert.Equal(t, "suspect", swapped.GetTag())
	assert.Equal(t, common.ChannelStatusManuallyDisabled, reload("down").Status)

	// The run as the overview tables read it.
	runs := detectionAPI(t, ListDetectionRuns, "/api/model_replacement/runs?limit=5", nil)
	require.Len(t, runs["runs"], 1)
	run := runs["runs"].([]any)[0].(map[string]any)
	assert.Equal(t, "schedule", run["trigger"])
	assert.Len(t, run["cells"], 5)
	for _, raw := range run["cells"].([]any) {
		cell := raw.(map[string]any)
		assert.Equal(t, cell["channel_id"] == float64(channels["swapped"].Id), cell["response_model_mismatch"], "only the swapped channel declared another model")
	}

	// One cell, round by round.
	cell := detectionAPI(t, GetDetectionCell, fmt.Sprintf("/api/model_replacement/runs/run-1/cell?channel_id=%d&model=gpt-4o-mini", channels["swapped"].Id), gin.Params{{Key: "id", Value: "run-1"}})
	attempts := cell["attempts"].([]any)
	require.Len(t, attempts, 2)
	first := attempts[0].(map[string]any)
	assert.Equal(t, true, first["answer_matched"])
	assert.Equal(t, "gpt-3.5-turbo", first["response_model"])
	assert.Equal(t, "mismatch", first["response_model_note"])
	assert.Equal(t, false, first["matched"])
	dated := detectionAPI(t, GetDetectionCell, fmt.Sprintf("/x?channel_id=%d&model=gpt-4o-mini", channels["dated"].Id), gin.Params{{Key: "id", Value: "run-1"}})
	assert.Equal(t, "loose_accepted", dated["attempts"].([]any)[0].(map[string]any)["response_model_note"])

	// A retest probes a disabled channel but leaves it alone.
	_, err = runDetection(context.Background(), "run-2", detectionTaskPayload{Manual: true, ChannelID: channels["down"].Id}, func(int, int) {})
	require.NoError(t, err)
	retest, err := model.GetDetectionRecord("run-2", channels["down"].Id)
	require.NoError(t, err)
	assert.True(t, retest.ActionsSkipped)
	assert.Equal(t, "retest", retest.RunTrigger)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, reload("down").Status)
}

func TestDetectionReadsFirstReleaseRecordsAndPurgesOldOnes(t *testing.T) {
	channels := setupDetectionTest(t)
	good := channels["good"]

	// A manual run stored by the first fork release, and a record past
	// retention.
	task := model.SystemTask{TaskID: "legacy-run", Type: model.SystemTaskTypeModelReplacementDetection, Status: model.SystemTaskStatusSucceeded, Payload: `{"manual":true}`}
	require.NoError(t, model.DB.Create(&task).Error)
	legacy := &model.ModelReplacementRecord{RunID: "legacy-run", ChannelID: good.Id, ChannelName: good.Name, Verdict: "replaced",
		ModelStats: `[{"model":"gpt-4o-mini","total":1,"matched":0,"mismatched":1,"failed":0,"match_percent":0,"mismatch_percent":100,"verdict":"replaced"}]`,
		Actions:    `[{"type":"set_tag","value":"suspect","amount":0,"changed":true}]`, StartedAt: time.Now().Unix() - 60, FinishedAt: time.Now().Unix() - 50}
	require.NoError(t, model.CreateModelReplacementRecord(legacy, []model.ModelReplacementAttempt{{Model: "gpt-4o-mini", Round: 1, Tries: 1, Success: true, Answer: "42", ErrorMessage: legacyResponseModelMismatch}}))
	old := &model.ModelReplacementRecord{RunID: "old-run", ChannelID: good.Id, Verdict: "passed", CreatedAt: time.Now().Add(-8 * 24 * time.Hour).Unix()}
	require.NoError(t, model.CreateModelReplacementRecord(old, []model.ModelReplacementAttempt{{Model: "gpt-4o-mini", Round: 1, Success: true}}))

	run := detectionAPI(t, GetDetectionRun, "/api/model_replacement/runs/legacy-run", gin.Params{{Key: "id", Value: "legacy-run"}})
	assert.Equal(t, "manual", run["trigger"], "the trigger of a first-release run comes from its task")
	cell := detectionAPI(t, GetDetectionCell, fmt.Sprintf("/x?channel_id=%d&model=gpt-4o-mini", good.Id), gin.Params{{Key: "id", Value: "legacy-run"}})
	attempt := cell["attempts"].([]any)[0].(map[string]any)
	assert.Equal(t, map[string]any{"answer_matched": true, "response_model_note": "mismatch", "response_model_ok": false, "error": ""},
		map[string]any{"answer_matched": attempt["answer_matched"], "response_model_note": attempt["response_model_note"], "response_model_ok": attempt["response_model_ok"], "error": attempt["error"]})

	_, err := runDetection(context.Background(), "run-1", detectionTaskPayload{}, func(int, int) {})
	require.NoError(t, err)
	var oldRecords, oldAttempts int64
	require.NoError(t, model.DB.Model(&model.ModelReplacementRecord{}).Where("run_id = ?", "old-run").Count(&oldRecords).Error)
	require.NoError(t, model.DB.Model(&model.ModelReplacementAttempt{}).Where("record_id = ?", old.ID).Count(&oldAttempts).Error)
	assert.Zero(t, oldRecords+oldAttempts, "records past the retention are deleted with their attempts")
	_, err = model.GetDetectionRecord("legacy-run", good.Id)
	assert.NoError(t, err, "recent records stay")
}
