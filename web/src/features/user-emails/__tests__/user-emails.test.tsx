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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { renderTemplate, type EmailTemplateSettings } from '../api'
import { AccountNoticeDialog } from '../components/account-notice-dialog'
import { UserEmailDrawer } from '../components/user-email-drawer'

const settings: EmailTemplateSettings = {
  templates: {
    suspended: {
      subject: '{{site_name}}: {{username}} suspended',
      html: '<p>Hi {{display_name}}</p>',
    },
    restored: { subject: 'Restored', html: '<p>Back</p>' },
  },
  defaults: {
    suspended: { subject: 'Suspended', html: '<p>Default</p>' },
    restored: { subject: 'Restored', html: '<p>Default</p>' },
  },
  variables: ['display_name', 'username', 'email', 'site_name', 'site_url'],
  site_name: 'Miao',
  site_url: 'https://miao.example',
}

const user = {
  id: 7,
  username: 'alice',
  display_name: '<b>Alice</b>',
  email: 'alice@example.com',
}

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/email_templates') {
      return { data: { success: true, data: settings } }
    }
    return { data: { success: true, data: [] } }
  })
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
})

function withClient(children: ReactNode) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

it('fills template variables, escaping them in the body only', () => {
  const rendered = renderTemplate(settings.templates.suspended, {
    display_name: '<b>Alice</b>',
    username: 'a&b',
    email: 'alice@example.com',
    site_name: 'Miao',
    site_url: 'https://miao.example',
  })
  expect(rendered.subject).toBe('Miao: a&b suspended')
  expect(rendered.html).toBe('<p>Hi &lt;b&gt;Alice&lt;/b&gt;</p>')
  expect(
    renderTemplate(
      { subject: '{{unknown}}', html: '' },
      {
        display_name: '',
        username: '',
        email: '',
        site_name: '',
        site_url: '',
      }
    ).subject
  ).toBe('{{unknown}}')
})

it('sends the account notice with the grant and no second verification', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: null } })
  const onOpenChange = vi.fn()
  render(
    withClient(
      <AccountNoticeDialog
        open
        onOpenChange={onOpenChange}
        kind='suspended'
        user={user}
        noticeToken='grant-1'
      />
    )
  )

  expect(await screen.findByText('Miao: alice suspended')).toBeVisible()
  expect(screen.getByText('Hi <b>Alice</b>')).toBeVisible()
  await userEvent.click(screen.getByRole('button', { name: 'Send notice' }))

  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
  expect(post).toHaveBeenCalledWith('/api/user/7/notice', {
    kind: 'suspended',
    notice_token: 'grant-1',
  })
})

it('sends an email only after verification bound to the recipient', async () => {
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: null } })
  const requestVerification = vi
    .fn()
    .mockResolvedValueOnce(null)
    .mockResolvedValue({ proof_token: 'proof-1' })
  render(
    withClient(
      <UserEmailDrawer
        open
        onOpenChange={vi.fn()}
        user={user}
        requestVerification={requestVerification}
      />
    )
  )

  await userEvent.type(await screen.findByLabelText('Subject'), 'Hello')
  const send = screen.getByRole('button', { name: 'Send' })
  await userEvent.click(send)
  await waitFor(() => expect(requestVerification).toHaveBeenCalledOnce())
  expect(requestVerification).toHaveBeenCalledWith(
    expect.objectContaining({
      scope: 'admin.user.email',
      context: { user_id: 7, email: 'alice@example.com' },
    })
  )
  expect(post).not.toHaveBeenCalled()

  await userEvent.click(send)
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/api/user/7/email',
      expect.objectContaining({ subject: 'Hello', images: [] }),
      { headers: { 'X-Security-Proof': 'proof-1' } }
    )
  )
})
