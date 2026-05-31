import { createFileRoute } from '@tanstack/react-router'
import { SignIn } from '@clerk/react'
import { Skeleton } from '@/components/ui/skeleton'

export const Route = createFileRoute('/clerk/(auth)/sign-in')({
  component: () => (
    <SignIn
      initialValues={{
        emailAddress: 'admin@superteam.local',
      }}
      fallback={<Skeleton className='h-120 w-100' />}
    />
  ),
})
