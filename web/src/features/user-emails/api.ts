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
import { api } from '@/lib/api'

export interface EmailImage {
  cid: string
  content_type: string
  content: string
}

export interface EmailDraft {
  subject: string
  html: string
  images: EmailImage[]
}

export interface EmailHistoryItem {
  time: number
  operator: string
  subject: string
  success: boolean
}

export type NoticeKind = 'suspended' | 'restored'
export type EmailTemplateKind = 'blank' | NoticeKind

export interface EmailTemplate {
  subject: string
  html: string
}

export type NoticeTemplates = Record<NoticeKind, EmailTemplate>

export interface EmailTemplateSettings {
  templates: NoticeTemplates
  defaults: NoticeTemplates
  variables: string[]
  site_name: string
  site_url: string
}

export type TemplateValues = {
  display_name: string
  username: string
  email: string
  site_name: string
  site_url: string
}

type ApiResponse<T> = { success: boolean; message?: string; data?: T }

export async function sendUserEmail(
  userId: number,
  draft: EmailDraft,
  proofToken: string
) {
  const res = await api.post<ApiResponse<null>>(
    `/api/user/${userId}/email`,
    draft,
    { headers: { 'X-Security-Proof': proofToken } }
  )
  return res.data
}

/** Sends the suspension or restore notice with the token the disable or
 * enable action returned, so no second verification is needed. */
export async function sendAccountNotice(
  userId: number,
  kind: NoticeKind,
  noticeToken: string
) {
  const res = await api.post<ApiResponse<null>>(`/api/user/${userId}/notice`, {
    kind,
    notice_token: noticeToken,
  })
  return res.data
}

export async function sendTestEmail(draft: EmailDraft) {
  const res = await api.post<ApiResponse<{ email: string }>>(
    '/api/user/self/email/test',
    draft
  )
  return res.data
}

export async function getEmailHistory(userId: number) {
  const res = await api.get<ApiResponse<EmailHistoryItem[]>>(
    `/api/user/${userId}/email/history`
  )
  return res.data
}

export async function getEmailTemplates() {
  const res = await api.get<ApiResponse<EmailTemplateSettings>>(
    '/api/email_templates'
  )
  return res.data
}

export async function updateEmailTemplates(templates: NoticeTemplates) {
  const res = await api.put<ApiResponse<{ templates: NoticeTemplates }>>(
    '/api/email_templates',
    { templates }
  )
  return res.data
}

const escapeHtml = (value: string) => {
  const node = document.createElement('span')
  node.textContent = value
  return node.innerHTML
}

/** Fills {{variable}} placeholders the same way the server does. */
export function renderTemplate(
  template: EmailTemplate,
  values: TemplateValues
): EmailTemplate {
  const fill = (text: string, escape: boolean) =>
    text.replaceAll(/\{\{\s*([a-z_]+)\s*\}\}/g, (match, key: string) => {
      if (!(key in values)) return match
      const value = values[key as keyof TemplateValues]
      return escape ? escapeHtml(value) : value
    })
  return {
    subject: fill(template.subject, false),
    html: fill(template.html, true),
  }
}

/** Moves uploaded data: images into CID attachments; links stay as they are. */
export function prepareDraft(subject: string, html: string): EmailDraft {
  const template = document.createElement('template')
  template.innerHTML = html
  const images: EmailImage[] = []
  template.content.querySelectorAll('img').forEach((image) => {
    const match = /^data:(image\/(?:png|jpeg|gif|webp));base64,(.+)$/.exec(
      image.getAttribute('src') ?? ''
    )
    if (!match) return
    const cid = `image-${images.length + 1}`
    images.push({ cid, content_type: match[1], content: match[2] })
    image.setAttribute('src', `cid:${cid}`)
  })
  return { subject: subject.trim(), html: template.innerHTML, images }
}
