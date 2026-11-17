/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export type Verdict = 'passed' | 'replaced' | 'failed' | 'not_tested'

export type ChannelStatus = 'enabled' | 'auto_disabled' | 'manual_disabled'

export type MatchMode = 'exact' | 'contains' | 'regex'

export type ResponseModelCheck = 'off' | 'exact' | 'loose'

export type ActionType =
  | 'add_group'
  | 'remove_group'
  | 'increase_priority'
  | 'decrease_priority'
  | 'set_priority'
  | 'increase_weight'
  | 'decrease_weight'
  | 'set_weight'
  | 'set_tag'
  | 'remove_tag'
  | 'auto_disable'
  | 'manual_disable'

/**
 * A channel action. In the editor `value` holds every parameter as text;
 * the server keeps priority and weight numbers in `amount`, which api.ts
 * converts. Applied actions from the server carry `amount` as stored.
 */
export interface DetectionAction {
  type: ActionType
  value: string
  amount?: number
}

export interface AppliedAction extends DetectionAction {
  changed: boolean
}

export interface IntervalWindow {
  start: string
  end: string
  interval_minutes: number
}

export interface MonitoredModel {
  model: string
  question: string
  expected_answer: string
  match_mode: MatchMode
  response_model_check: ResponseModelCheck
  retry_times: number
  excluded_channel_ids: number[]
}

export interface DetectionSettings {
  enabled: boolean
  interval_minutes: number
  interval_windows: IntervalWindow[]
  retention_days: number
  rounds: number
  concurrency: number
  timeout_seconds: number
  threshold: number
  models: MonitoredModel[]
  pass_actions: DetectionAction[]
  fail_actions: DetectionAction[]
  request_failure_actions: DetectionAction[]
}

export type ActionListKey =
  | 'pass_actions'
  | 'fail_actions'
  | 'request_failure_actions'

export interface SettingsResponse {
  settings: DetectionSettings
  server_time_zone: string
}

export interface ChannelInfo {
  id: number
  name: string
  status: ChannelStatus
  groups: string[]
  priority: number
  weight: number
  tag: string
}

export interface RunSummary {
  id: string
  trigger: 'schedule' | 'manual' | 'retest'
  channel_id: number | null
  started_at: number
  finished_at: number
  duration_ms: number
  counts: { passed: number; replaced: number; failed: number }
}

export interface RunCell {
  channel_id: number
  model: string
  verdict: Verdict
  match_percent: number
  response_model_mismatch: boolean
}

export interface ChannelResult {
  channel_id: number
  verdict: Verdict
  actions: AppliedAction[]
  skipped_actions: boolean
}

export interface DetectionRun extends RunSummary {
  cells: RunCell[]
  channel_results: ChannelResult[]
}

export interface ActiveRun {
  id: string
  trigger: RunSummary['trigger']
  channel_id: number | null
  started_at: number
  done: number
  total: number
}

export interface DetectionOverview {
  enabled: boolean
  interval_minutes: number
  server_time_zone: string
  next_run_at: number | null
  last_run: RunSummary | null
  active_run: ActiveRun | null
  counts: {
    healthy: number
    suspected: number
    failed: number
    not_tested: number
    in_scope: number
  }
  rounds: number
  models: string[]
}

export interface LatestResult {
  run_id: string
  started_at: number
  finished_at: number
  trigger: RunSummary['trigger']
  verdict: Verdict
  avg_latency_ms: number
  cells: Omit<RunCell, 'channel_id'>[]
  actions: AppliedAction[]
  skipped_actions: boolean
}

export interface DetectionChannel extends ChannelInfo {
  in_scope: boolean
  monitored_models: string[]
  latest: LatestResult | null
}

export interface CandidateModel {
  model: string
  channels: ChannelInfo[]
}

export type ResponseModelNote =
  | 'not_checked'
  | 'exact'
  | 'loose_accepted'
  | 'mismatch'

export interface Attempt {
  round: number
  answer: string
  answer_matched: boolean
  response_model: string
  response_model_ok: boolean
  response_model_note: ResponseModelNote
  matched: boolean
  latency_ms: number
  tries: number
  error: string
}

export interface CellDetail {
  run: RunSummary
  channel: ChannelInfo
  model: string
  verdict: Verdict
  match_percent: number
  channel_verdict: Verdict
  question: string
  expected_answer: string
  match_mode: MatchMode
  response_model_check: ResponseModelCheck
  attempts: Attempt[]
  actions: AppliedAction[]
  skipped_actions: boolean
}

export interface TrialRequest {
  channel_id: number
  model: string
  question: string
  expected_answer: string
  match_mode: MatchMode
  response_model_check: ResponseModelCheck
}

export interface TrialResult {
  channel: ChannelInfo
  model: string
  answer: string
  answer_matched: boolean
  response_model: string
  response_model_ok: boolean
  response_model_note: ResponseModelNote
  latency_ms: number
  error: string
  verdict: Verdict
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}
