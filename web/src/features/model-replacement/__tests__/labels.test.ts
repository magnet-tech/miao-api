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
import { expect, it } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'

import {
  ACTION_CATEGORIES,
  ACTION_DEFS,
  ACTION_LISTS,
  ACTION_VALUE_PLACEHOLDERS,
  CHANNEL_STATUS,
  MATCH_MODE_LABELS,
  MODEL_EDITOR_ERRORS,
  RESPONSE_CHECK_HINTS,
  RESPONSE_CHECK_LABELS,
  RESPONSE_MODEL_NOTE_LABELS,
  TRIGGER_LABELS,
  VERDICT_STYLES,
} from '../lib/constants'

const locales: Record<string, Record<string, string>> = {
  en: en.translation,
  zh: zh.translation,
  'zh-TW': zhTW.translation,
  fr: fr.translation,
  ru: ru.translation,
  ja: ja.translation,
  vi: vi.translation,
}

// The page translates these keys at runtime with t(variable), so nothing
// else notices a key that no locale file has: it shows in English everywhere.
it('gives every model replacement label a translation in each locale', () => {
  const keys = [
    ...Object.values(VERDICT_STYLES).map((style) => style.labelKey),
    ...Object.values(CHANNEL_STATUS).map((status) => status.labelKey),
    ...Object.values(MATCH_MODE_LABELS),
    ...Object.values(RESPONSE_CHECK_LABELS),
    ...Object.values(RESPONSE_CHECK_HINTS),
    ...Object.values(RESPONSE_MODEL_NOTE_LABELS),
    ...Object.values(TRIGGER_LABELS),
    ...Object.values(MODEL_EDITOR_ERRORS),
    ...Object.values(ACTION_DEFS).flatMap((def) => [
      def.labelKey,
      def.category,
    ]),
    ...ACTION_CATEGORIES,
    ...Object.values(ACTION_VALUE_PLACEHOLDERS),
    ...ACTION_LISTS.flatMap((list) => [list.titleKey, list.descriptionKey]),
  ]

  const missing = Object.entries(locales).flatMap(([language, messages]) =>
    keys.filter((key) => !messages[key]).map((key) => `${language}: ${key}`)
  )
  expect(missing).toEqual([])
})
