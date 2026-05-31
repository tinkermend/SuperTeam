import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthProvider } from './auth-provider'
import { useAuth } from './use-auth'

function createDeferredResponse() {
  let resolve!: (response: Response) => void
  const promise = new Promise<Response>((promiseResolve) => {
    resolve = promiseResolve
  })

  return { promise, resolve }
}

function AuthStatus() {
  const { isAuthenticated, isLoading, user } = useAuth()

  if (isLoading) {
    return <p>Loading</p>
  }

  return (
    <p>
      {isAuthenticated ? `Signed in as ${user?.username}` : 'Signed out'}
    </p>
  )
}

function LoginProbe() {
  const { login } = useAuth()

  return (
    <button onClick={() => void login({ username: 'new', password: 'secret' })}>
      Login
    </button>
  )
}

function FailedLoginProbe() {
  const { login } = useAuth()

  return (
    <button
      onClick={() => {
        void login({ username: 'new', password: 'wrong' }).catch(() => {})
      }}
    >
      Login
    </button>
  )
}

describe('AuthProvider', () => {
  it('clears the authenticated user when focus refresh receives 401', async () => {
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            user: {
              id: 1,
              username: 'admin',
              status: 'active',
            },
          }),
          {
            status: 200,
            headers: {
              'content-type': 'application/json',
            },
          }
        )
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: 'unauthorized' }), {
          status: 401,
          headers: {
            'content-type': 'application/json',
          },
        })
      )

    const screen = await render(
      <AuthProvider apiBaseUrl='http://control-plane.local' fetcher={fetcher}>
        <AuthStatus />
      </AuthProvider>
    )

    await expect.element(screen.getByText('Signed in as admin')).toBeVisible()

    window.dispatchEvent(new Event('focus'))

    await expect.element(screen.getByText('Signed out')).toBeVisible()
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('keeps a newer login user when the initial current-user request resolves later', async () => {
    const initialMe = createDeferredResponse()
    let currentUserCalls = 0
    const fetcher = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)

      if (url.endsWith('/api/auth/login')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: {
                id: 2,
                username: 'new',
                status: 'active',
              },
            }),
            {
              status: 200,
              headers: {
                'content-type': 'application/json',
              },
            }
          )
        )
      }

      currentUserCalls += 1
      if (currentUserCalls === 1) {
        return initialMe.promise
      }

      return Promise.resolve(
        new Response(
          JSON.stringify({
            user: {
              id: 1,
              username: 'old',
              status: 'active',
            },
          }),
          {
            status: 200,
            headers: {
              'content-type': 'application/json',
            },
          }
        )
      )
    })

    const screen = await render(
      <AuthProvider apiBaseUrl='http://control-plane.local' fetcher={fetcher}>
        <AuthStatus />
        <LoginProbe />
      </AuthProvider>
    )

    await screen.getByRole('button', { name: 'Login' }).click()
    await expect.element(screen.getByText('Signed in as new')).toBeVisible()

    initialMe.resolve(
      new Response(
        JSON.stringify({
          user: {
            id: 1,
            username: 'old',
            status: 'active',
          },
        }),
        {
          status: 200,
          headers: {
            'content-type': 'application/json',
          },
        }
      )
    )

    await expect.element(screen.getByText('Signed in as new')).toBeVisible()
    expect(screen.getByText('Signed in as old')).not.toBeInTheDocument()
  })

  it('clears loading when login fails after superseding a slow initial current-user request', async () => {
    const initialMe = createDeferredResponse()
    const fetcher = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)

      if (url.endsWith('/api/auth/login')) {
        return Promise.resolve(
          new Response(JSON.stringify({ error: 'invalid_credentials' }), {
            status: 401,
            headers: {
              'content-type': 'application/json',
            },
          })
        )
      }

      return initialMe.promise
    })

    const screen = await render(
      <AuthProvider apiBaseUrl='http://control-plane.local' fetcher={fetcher}>
        <AuthStatus />
        <FailedLoginProbe />
      </AuthProvider>
    )

    await screen.getByRole('button', { name: 'Login' }).click()

    await expect.element(screen.getByText('Signed out')).toBeVisible()
    expect(screen.getByText('Loading')).not.toBeInTheDocument()

    initialMe.resolve(
      new Response(
        JSON.stringify({
          user: {
            id: 1,
            username: 'old',
            status: 'active',
          },
        }),
        {
          status: 200,
          headers: {
            'content-type': 'application/json',
          },
        }
      )
    )

    await expect.element(screen.getByText('Signed out')).toBeVisible()
    expect(screen.getByText('Signed in as old')).not.toBeInTheDocument()
  })
})
