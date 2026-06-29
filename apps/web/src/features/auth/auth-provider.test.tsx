import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthProvider } from './auth-provider'
import { useAuth } from './use-auth'

type TestCurrentUserResponse = {
  user: {
    id: number;
    status: string;
    username: string;
  };
};

function createDeferredJsonResponse<T>() {
  let resolve!: (payload: T) => void
  const payloadPromise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve
  })

  return {
    response: () =>
      payloadPromise.then(
        (payload) =>
          new Response(JSON.stringify(payload), {
            status: 200,
            headers: {
              'content-type': 'application/json',
            },
          })
      ),
    resolve,
  }
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
    const initialMe = createDeferredJsonResponse<TestCurrentUserResponse>()
    const loginMe = createDeferredJsonResponse<TestCurrentUserResponse>()
    let didLogin = false
    const fetcher = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)

      if (url.endsWith('/api/auth/login')) {
        didLogin = true
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

      return didLogin ? loginMe.response() : initialMe.response()
    })

    const screen = await render(
      <AuthProvider apiBaseUrl='http://control-plane.local' fetcher={fetcher}>
        <AuthStatus />
        <LoginProbe />
      </AuthProvider>
    )

    await screen.getByRole('button', { name: 'Login' }).click()
    loginMe.resolve({
      user: {
        id: 2,
        username: 'new',
        status: 'active',
      },
    })

    initialMe.resolve({
      user: {
        id: 1,
        username: 'old',
        status: 'active',
      },
    })

    await expect.element(screen.getByText('Signed in as new')).toBeVisible()
    expect(screen.getByText('Signed in as old')).not.toBeInTheDocument()
  })

  it('waits for current user confirmation before completing login', async () => {
    const loginMe = createDeferredJsonResponse<TestCurrentUserResponse>()
    let didLogin = false
    const fetcher = vi.fn((input: RequestInfo | URL) => {
      const url = String(input)

      if (url.endsWith('/api/auth/login')) {
        didLogin = true
        return Promise.resolve(
          new Response(JSON.stringify({ user: { id: 2, username: 'new', status: 'active' } }), {
            status: 200,
            headers: {
              'content-type': 'application/json',
            },
          })
        )
      }

      if (didLogin) {
        return loginMe.response()
      }

      return Promise.resolve(
        new Response(JSON.stringify({ error: 'unauthorized' }), {
          status: 401,
          headers: {
            'content-type': 'application/json',
          },
        })
      )
    })

    const screen = await render(
      <AuthProvider apiBaseUrl='http://control-plane.local' fetcher={fetcher}>
        <AuthStatus />
        <LoginProbe />
      </AuthProvider>
    )

    await expect.element(screen.getByText('Signed out')).toBeVisible()
    await screen.getByRole('button', { name: 'Login' }).click()

    expect(screen.getByText('Signed in as new')).not.toBeInTheDocument()

    loginMe.resolve({ user: { id: 2, username: 'new', status: 'active' } })

    await expect.element(screen.getByText('Signed in as new')).toBeVisible()
  })

  it('clears loading when login fails after superseding a slow initial current-user request', async () => {
    const initialMe = createDeferredJsonResponse<TestCurrentUserResponse>()
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

      return initialMe.response()
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

    initialMe.resolve({
      user: {
        id: 1,
        username: 'old',
        status: 'active',
      },
    })

    await expect.element(screen.getByText('Signed out')).toBeVisible()
    expect(screen.getByText('Signed in as old')).not.toBeInTheDocument()
  })
})
