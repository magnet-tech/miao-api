package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Model replacement detection keeps one record per channel and run, with the
// probes behind it. The tables and their original columns are those of the
// first fork release; columns added since are empty on its rows.

const (
	DetectionVerdictPassed   = "passed"
	DetectionVerdictReplaced = "replaced"
	DetectionVerdictFailed   = "failed"

	DetectionTriggerSchedule = "schedule"
	DetectionTriggerManual   = "manual"
	DetectionTriggerRetest   = "retest"
)

// DetectionAppliedAction is a configured action as it ran on a channel;
// Changed is false when the channel already matched it.
type DetectionAppliedAction struct {
	Type    string `json:"type"`
	Value   string `json:"value"`
	Amount  int64  `json:"amount"`
	Changed bool   `json:"changed"`
}

// DetectionModelStat is the result of one monitored model of one channel in
// one run. Percentages are 0-100 over the model's rounds.
type DetectionModelStat struct {
	Model           string  `json:"model"`
	Total           int     `json:"total"`
	Matched         int     `json:"matched"`
	Mismatched      int     `json:"mismatched"`
	Failed          int     `json:"failed"`
	MatchPercent    float64 `json:"match_percent"`
	MismatchPercent float64 `json:"mismatch_percent"`
	Verdict         string  `json:"verdict"`
	// ResponseModelMismatch is set when a round declared another model.
	ResponseModelMismatch bool `json:"response_model_mismatch,omitempty"`
}

// ModelReplacementRecord is the result of one run for one channel.
type ModelReplacementRecord struct {
	ID           int64   `json:"id" gorm:"primaryKey"`
	RunID        string  `json:"run_id" gorm:"type:varchar(64);index"`
	ChannelID    int     `json:"channel_id" gorm:"index:idx_model_replacement_records_channel,priority:1"`
	ChannelName  string  `json:"channel_name" gorm:"type:varchar(255)"`
	Verdict      string  `json:"verdict" gorm:"type:varchar(16)"`
	MatchPercent float64 `json:"match_percent"`
	Actions      string  `json:"-" gorm:"type:text"` // JSON []DetectionAppliedAction
	ErrorMessage string  `json:"error_message" gorm:"type:text"`
	ModelStats   string  `json:"-" gorm:"type:text"` // JSON []DetectionModelStat
	AvgLatencyMs int64   `json:"avg_latency_ms" gorm:"bigint"`
	StartedAt    int64   `json:"started_at" gorm:"bigint"`
	FinishedAt   int64   `json:"finished_at" gorm:"bigint"`
	CreatedAt    int64   `json:"created_at" gorm:"bigint;index;index:idx_model_replacement_records_channel,priority:2"`
	// RunTrigger is schedule, manual or retest; empty on first-release rows.
	RunTrigger string `json:"run_trigger" gorm:"type:varchar(16)"`
	// ActionsSkipped marks a channel that was not enabled, so no action ran.
	ActionsSkipped bool `json:"actions_skipped"`
}

// ModelReplacementAttempt is one probe of one model in one round, with its
// retries.
type ModelReplacementAttempt struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	RecordID     int64  `json:"record_id" gorm:"index"`
	Model        string `json:"model" gorm:"type:varchar(255)"`
	Round        int    `json:"round" gorm:"column:round_index"`
	Tries        int    `json:"tries"`
	Success      bool   `json:"success"`
	Matched      bool   `json:"matched"`
	Answer       string `json:"answer" gorm:"type:text"`
	ErrorMessage string `json:"error_message" gorm:"type:text"`
	LatencyMs    int64  `json:"latency_ms" gorm:"bigint"`
	TestedAt     int64  `json:"tested_at" gorm:"bigint"`
	// The answer and the declared response model judged separately;
	// ResponseModelNote is empty on first-release rows.
	AnswerMatched     bool   `json:"answer_matched"`
	ResponseModel     string `json:"response_model" gorm:"type:varchar(255)"`
	ResponseModelNote string `json:"response_model_note" gorm:"type:varchar(16)"`
}

func (record *ModelReplacementRecord) DecodeModelStats() []DetectionModelStat {
	stats := []DetectionModelStat{}
	if record.ModelStats != "" {
		if err := common.UnmarshalJsonStr(record.ModelStats, &stats); err != nil {
			common.SysError(common.LogText("failed to decode model replacement stats of record %d: %s", record.ID, err.Error()))
		}
	}
	return stats
}

func (record *ModelReplacementRecord) DecodeActions() []DetectionAppliedAction {
	actions := []DetectionAppliedAction{}
	if record.Actions != "" {
		if err := common.UnmarshalJsonStr(record.Actions, &actions); err != nil {
			common.SysError(common.LogText("failed to decode model replacement actions of record %d: %s", record.ID, err.Error()))
		}
	}
	return actions
}

// CreateModelReplacementRecord stores a record with its attempts, so a
// result never appears without its details.
func CreateModelReplacementRecord(record *ModelReplacementRecord, attempts []ModelReplacementAttempt) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		if len(attempts) == 0 {
			return nil
		}
		for i := range attempts {
			attempts[i].RecordID = record.ID
		}
		return tx.CreateInBatches(attempts, 100).Error
	})
}

// DetectionRun is a run as its records describe it.
type DetectionRun struct {
	RunID      string
	StartedAt  int64
	FinishedAt int64
	LastID     int64
}

// ListDetectionRuns returns the newest runs, newest first, and how many runs
// there are.
func ListDetectionRuns(limit int) ([]DetectionRun, int64, error) {
	var runs []DetectionRun
	err := DB.Model(&ModelReplacementRecord{}).
		Select("run_id, MIN(started_at) AS started_at, MAX(finished_at) AS finished_at, MAX(id) AS last_id").
		Group("run_id").Order("last_id DESC").Limit(limit).Scan(&runs).Error
	if err != nil {
		return nil, 0, err
	}
	var total int64
	err = DB.Model(&ModelReplacementRecord{}).Distinct("run_id").Count(&total).Error
	return runs, total, err
}

// GetDetectionRecordsOfRuns returns the records of the given runs.
func GetDetectionRecordsOfRuns(runIDs []string) ([]*ModelReplacementRecord, error) {
	var records []*ModelReplacementRecord
	if len(runIDs) == 0 {
		return records, nil
	}
	err := DB.Where("run_id IN ?", runIDs).Order("id").Find(&records).Error
	return records, err
}

// GetLatestDetectionRecords returns the newest record of every channel.
func GetLatestDetectionRecords() ([]*ModelReplacementRecord, error) {
	var records []*ModelReplacementRecord
	latest := DB.Model(&ModelReplacementRecord{}).Select("MAX(id)").Group("channel_id")
	err := DB.Where("id IN (?)", latest).Find(&records).Error
	return records, err
}

// GetDetectionRecord returns the record of a channel in a run.
func GetDetectionRecord(runID string, channelID int) (*ModelReplacementRecord, error) {
	var record ModelReplacementRecord
	err := DB.Where("run_id = ? AND channel_id = ?", runID, channelID).Order("id DESC").First(&record).Error
	return &record, err
}

// GetDetectionAttempts returns a record's probes of one model, by round.
func GetDetectionAttempts(recordID int64, modelName string) ([]ModelReplacementAttempt, error) {
	var attempts []ModelReplacementAttempt
	err := DB.Where("record_id = ? AND model = ?", recordID, modelName).Order("round_index, id").Find(&attempts).Error
	return attempts, err
}

// PurgeDetectionRecords deletes, with their attempts, the records created
// before the timestamp and those of deleted channels.
func PurgeDetectionRecords(before int64) error {
	const purged = "created_at < ? OR channel_id NOT IN (?)"
	return DB.Transaction(func(tx *gorm.DB) error {
		channels := tx.Model(&Channel{}).Select("id")
		ids := tx.Model(&ModelReplacementRecord{}).Select("id").Where(purged, before, channels)
		if err := tx.Where("record_id IN (?)", ids).Delete(&ModelReplacementAttempt{}).Error; err != nil {
			return err
		}
		return tx.Where(purged, before, tx.Model(&Channel{}).Select("id")).Delete(&ModelReplacementRecord{}).Error
	})
}

// UpdateChannelRouting writes the groups, priority, weight and tag that
// detection actions produced and rebuilds the channel's abilities from them,
// in one transaction with the channel row locked. A nil tag clears it.
// Callers refresh the channel cache.
func UpdateChannelRouting(channelID int, group string, priority int64, weight uint, tag *string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, "id = ?", channelID).Error; err != nil {
			return err
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).Updates(map[string]any{
			"group": group, "priority": priority, "weight": weight, "tag": tag,
		}).Error; err != nil {
			return err
		}
		channel.Group, channel.Priority, channel.Weight, channel.Tag = group, &priority, &weight, tag
		return channel.UpdateAbilities(tx)
	})
}
