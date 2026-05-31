import { createFileRoute, redirect } from '@tanstack/react-router'

type SignInSearch = {
  redirect?: string
}

export const Route = createFileRoute('/(auth)/sign-in')({
  validateSearch: (search: Record<string, unknown>): SignInSearch =>
    typeof search.redirect === 'string' ? { redirect: search.redirect } : {},
  beforeLoad: ({ search }) => {
    throw redirect({ to: '/login', search: { redirect: search.redirect } })
  },
})
