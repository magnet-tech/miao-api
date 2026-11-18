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
import { useState } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { EmailTemplatesSection } from '../email-templates-section'

const templates = {
  suspended: { subject: 'Suspended {{username}}', html: '<p>Hi</p>' },
  restored: { subject: 'Restored', html: '<p>Back</p>' },
}

let client: QueryClient
beforeEach(() => {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        templates,
        defaults: {
          suspended: { subject: 'Default suspended', html: '<p>D</p>' },
          restored: { subject: 'Default restored', html: '<p>D</p>' },
        },
        saved: true,
        variables: [
          'display_name',
          'username',
          'email',
          'site_name',
          'site_url',
        ],
        site_name: 'Miao',
        site_url: 'https://miao.example',
      },
    },
  })
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
})

function Page() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <QueryClientProvider client={client}>
      <div ref={setActions} />
      <SettingsPageProvider actionsContainer={actions}>
        <EmailTemplatesSection />
      </SettingsPageProvider>
    </QueryClientProvider>
  )
}

it('saves an edited template and keeps save disabled until something changes', async () => {
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: { templates } } })
  render(<Page />)

  const subject = await screen.findByLabelText('Subject')
  expect(screen.getByText('Suspended alice')).toBeVisible()
  const save = screen.getByRole('button', { name: 'Save templates' })
  expect(save).toBeDisabled()

  await userEvent.clear(subject)
  await userEvent.type(subject, 'Paused {{{{username}}')
  expect(screen.getByText('Paused alice')).toBeVisible()
  await userEvent.click(save)

  await waitFor(() =>
    expect(put).toHaveBeenCalledWith('/api/email_templates', {
      templates: {
        ...templates,
        suspended: { ...templates.suspended, subject: 'Paused {{username}}' },
      },
    })
  )
})

it('restores the built-in template into the draft without saving', async () => {
  const put = vi.spyOn(api, 'put')
  render(<Page />)

  await screen.findByLabelText('Subject')
  await userEvent.click(screen.getByRole('button', { name: 'Restore default' }))

  expect(screen.getByLabelText('Subject')).toHaveValue('Default suspended')
  expect(
    screen.getByRole('button', { name: 'Save templates' })
  ).not.toBeDisabled()
  expect(put).not.toHaveBeenCalled()
})
