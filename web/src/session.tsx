import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect } from 'react'
import { api, json, send } from './api'
import type { components } from './api.gen'

const activityEvery = 60_000

export class Unauthenticated extends Error {}

type Challenge = components['schemas']['MfaChallenge']

let loginChallenge: Challenge | undefined

export function rememberChallenge(challenge: Challenge | undefined) {
  loginChallenge = challenge
}

export function pendingChallenge() {
  return loginChallenge?.challenge
}

export function useChallengeExpiry() {
  const navigate = useNavigate()
  useEffect(() => {
    if (!loginChallenge) {
      return
    }
    const expiry = setTimeout(() => {
      if (loginChallenge) {
        rememberChallenge(undefined)
        void navigate({ to: '/login', search: { notice: 'verification-interrupted' } })
      }
    }, Date.parse(loginChallenge.expiresAt) - Date.now())
    return () => clearTimeout(expiry)
  }, [navigate])
}

export const meQuery = queryOptions({
  queryKey: ['me'],
  queryFn: async () => {
    const { data, response } = await api.GET('/auth/me')
    if (response.status === 401) {
      throw new Unauthenticated()
    }
    if (!data) {
      throw new Error(`me: ${response.status}`)
    }
    return data
  },
})

export function useMe() {
  return useQuery(meQuery)
}

export function useShellMe() {
  return useQuery({ ...meQuery, refetchOnMount: false })
}

export function useElevated() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  return useCallback(async () => {
    rememberChallenge(undefined)
    queryClient.clear()
    await navigate({ to: '/' })
  }, [navigate, queryClient])
}

export function useSessionClosed() {
  const navigate = useNavigate()
  return useCallback(() => {
    void navigate({ to: '/login', search: { notice: 'session-closed' } })
  }, [navigate])
}

export function useActivity(idleTimeoutSeconds: number | null | undefined) {
  const closed = useSessionClosed()
  useEffect(() => {
    if (idleTimeoutSeconds === undefined) {
      return
    }
    let active = true
    let lastReport = Number.NEGATIVE_INFINITY
    let idle: ReturnType<typeof setTimeout> | undefined
    const arm = () => {
      clearTimeout(idle)
      if (idleTimeoutSeconds !== null) {
        idle = setTimeout(closed, idleTimeoutSeconds * 1000)
      }
    }
    const gesture = () => {
      arm()
      if (Date.now() - lastReport < activityEvery) {
        return
      }
      lastReport = Date.now()
      void send(() => api.POST('/auth/activity', { headers: json })).then(({ response }) => {
        if (active && response?.status === 401) {
          closed()
        }
      })
    }
    arm()
    document.addEventListener('pointerdown', gesture)
    document.addEventListener('keydown', gesture)
    return () => {
      active = false
      clearTimeout(idle)
      document.removeEventListener('pointerdown', gesture)
      document.removeEventListener('keydown', gesture)
    }
  }, [idleTimeoutSeconds, closed])
}
