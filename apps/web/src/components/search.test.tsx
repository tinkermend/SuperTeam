import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { Search } from './search'

vi.mock('@/context/search-provider', () => ({
  useSearch: () => ({ setOpen: vi.fn() }),
}))

describe('Search', () => {
  it('uses the SuperTeam command-search placeholder by default', async () => {
    const { getByRole } = await render(<Search />)

    await expect
      .element(getByRole('button', { name: /搜索任务、数字员工、能力/ }))
      .toBeInTheDocument()
  })
})
