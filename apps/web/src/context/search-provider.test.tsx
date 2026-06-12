import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SearchProvider } from '@/context/search-provider'

const COMMAND_MENU_PLACEHOLDER = 'Type a command or search...'

;(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean })
  .IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({
  navigate: vi.fn(),
  setTheme: vi.fn(),
}))

vi.mock('@/components/ui/command', async () => {
  const React = await import('react')
  type CommandSearchContextValue = {
    search: string
    setSearch: (search: string) => void
  }
  const CommandSearchContext = React.createContext<CommandSearchContextValue>({
    search: '',
    setSearch: () => {},
  })
  const textFromChildren = (children: React.ReactNode): string =>
    React.Children.toArray(children)
      .map((child) => {
        if (typeof child === 'string' || typeof child === 'number') {
          return String(child)
        }
        if (React.isValidElement<{ children?: React.ReactNode }>(child)) {
          return textFromChildren(child.props.children)
        }
        return ''
      })
      .join('')

  return {
    CommandDialog: ({
      children,
      open,
    }: {
      children: React.ReactNode
      open: boolean
    }) => {
      const [search, setSearch] = React.useState('')
      if (!open) {
        return null
      }
      return (
        <CommandSearchContext value={{ search, setSearch }}>
          <div>{children}</div>
        </CommandSearchContext>
      )
    },
    CommandEmpty: ({ children }: { children: React.ReactNode }) => {
      const { search } = React.useContext(CommandSearchContext)
      return search ? <div>{children}</div> : null
    },
    CommandGroup: ({
      children,
      heading,
    }: {
      children: React.ReactNode
      heading: string
    }) => (
      <section>
        <div>{heading}</div>
        {children}
      </section>
    ),
    CommandInput: ({ placeholder }: { placeholder: string }) => {
      const { search, setSearch } = React.useContext(CommandSearchContext)
      return (
        <input
          placeholder={placeholder}
          value={search}
          onChange={(event) => setSearch(event.currentTarget.value)}
        />
      )
    },
    CommandItem: ({
      children,
      onSelect,
      value,
    }: {
      children: React.ReactNode
      onSelect?: () => void
      value?: string
    }) => {
      const { search } = React.useContext(CommandSearchContext)
      const itemText = value ?? textFromChildren(children)
      if (search && !itemText.includes(search)) {
        return null
      }
      return (
        <button type="button" onClick={onSelect}>
          {children}
        </button>
      )
    },
    CommandList: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    CommandSeparator: () => <hr />,
  }
})

vi.mock('@/components/ui/scroll-area', () => ({
  ScrollArea: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => mocks.navigate,
  }
})

vi.mock('@/context/theme-provider', () => ({
  useTheme: () => ({ setTheme: mocks.setTheme }),
}))

type ShortcutModifier = 'Control' | 'Meta'

let root: Root | null = null
let container: HTMLDivElement | null = null

async function renderWithSearchProvider() {
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)

  await act(async () => {
    root?.render(<SearchProvider>{null}</SearchProvider>)
  })
}

function queryByPlaceholder(placeholder: string) {
  return document.querySelector<HTMLInputElement>(`[placeholder="${placeholder}"]`)
}

function queryByText(text: string) {
  return Array.from(document.body.querySelectorAll<HTMLElement>('*')).find(
    (element) =>
      element.textContent === text &&
      Array.from(element.children).every((child) => child.textContent !== text)
  )
}

function getByText(text: string) {
  const element = queryByText(text)
  if (!element) {
    throw new Error(`Unable to find text: ${text}`)
  }
  return element
}

/**
 * Open the palette by shortcut, retrying while the keydown listener may not be mounted yet.
 * Waits between attempts so a successful toggle is not immediately undone by a second chord.
 */
async function openCommandPalette(
  modifier: ShortcutModifier = 'Control'
) {
  await vi.waitFor(
    async () => {
      const isCommandPaletteOpen = queryByPlaceholder(COMMAND_MENU_PLACEHOLDER) !== null

      if (!isCommandPaletteOpen) {
        await act(async () => {
          document.dispatchEvent(
            new KeyboardEvent('keydown', {
              bubbles: true,
              ctrlKey: modifier === 'Control',
              key: 'k',
              metaKey: modifier === 'Meta',
            })
          )
        })
      }

      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).not.toBeNull()
    },
    { interval: 50, timeout: 5000 }
  )
}

describe('SearchProvider and CommandMenu', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(async () => {
    await act(async () => {
      root?.unmount()
    })
    container?.remove()
    root = null
    container = null
    document.body.innerHTML = ''
  })

  it('renders the command palette when the palette is open', async () => {
    await renderWithSearchProvider()

    await openCommandPalette()

    expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).not.toBeNull()
    expect(queryByText('Theme')).not.toBeNull()
    expect(queryByText('Light')).not.toBeNull()
    expect(queryByText('Dark')).not.toBeNull()
    expect(queryByText('System')).not.toBeNull()
    expect(queryByText('工作台')).not.toBeNull()
  })

  it('does not show the dialog content when search is closed', async () => {
    await renderWithSearchProvider()

    expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).toBeNull()
  })

  it.each([
    ['Ctrl', 'Control'],
    ['Cmd', 'Meta'],
  ] as const)(
    'opens the command menu when %s + K is pressed',
    async (_label, modifier) => {
      await renderWithSearchProvider()

      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).toBeNull()

      await openCommandPalette(modifier)

      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).not.toBeNull()
    }
  )

  it('navigates to a top-level route and closes the palette when a nav item is selected', async () => {
    await renderWithSearchProvider()

    await openCommandPalette()

    await act(async () => {
      getByText('任务发起').click()
    })

    expect(mocks.navigate).toHaveBeenCalledWith({ to: '/task-launches' })
    await vi.waitFor(() => {
      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).toBeNull()
    })
  })

  it('navigates to another SuperTeam route from the command palette', async () => {
    await renderWithSearchProvider()

    await openCommandPalette()

    await act(async () => {
      getByText('审批中心').click()
    })

    expect(mocks.navigate).toHaveBeenCalledWith({ to: '/approvals' })
    await vi.waitFor(() => {
      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).toBeNull()
    })
  })

  it('applies theme and closes the palette when a theme command is chosen', async () => {
    await renderWithSearchProvider()

    await openCommandPalette()

    await act(async () => {
      getByText('Dark').click()
    })

    expect(mocks.setTheme).toHaveBeenCalledWith('dark')
    await vi.waitFor(() => {
      expect(queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)).toBeNull()
    })
  })

  it('shows empty state when the filter matches nothing', async () => {
    await renderWithSearchProvider()

    await openCommandPalette()

    await act(async () => {
      const input = queryByPlaceholder(COMMAND_MENU_PLACEHOLDER)
      if (!input) {
        throw new Error('Command menu input was not found')
      }
      input.value = 'zzzz-no-match-xxxx'
      input.dispatchEvent(new InputEvent('input', { bubbles: true }))
    })

    await vi.waitFor(() => {
      expect(queryByText('No results found.')).not.toBeNull()
    })
  })
})
