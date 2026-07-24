import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { Search } from './search'

vi.mock('@/context/search-provider', () => ({
  useSearch: () => ({ setOpen: vi.fn() })
}))

describe('Search', () => {
  it('uses the SuperTeam command-search placeholder by default', async () => {
    const { getByRole } = await render(<Search />)

    const searchButton = getByRole('button', { name: /搜索任务、数字员工、能力/ })
    await expect.element(searchButton).toBeInTheDocument()
    await expect.element(searchButton).toHaveClass('bg-[var(--shell-control)]')
    await expect.element(searchButton).toHaveClass('backdrop-blur-md')
    await expect.element(searchButton).toHaveClass('text-ink-2')
  })
})
