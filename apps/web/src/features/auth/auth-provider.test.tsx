import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthProvider } from './auth-provider'
import { useAuth } from './use-auth'

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
})
