import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect, useRef } from 'react'
import { api, json } from './api'

const activityEvery = 60_000

export class Unauthenticated extends Error {}

export function useMe() {
  return useQuery({
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
}

export function useSessionClosed() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  return useCallback(() => {
    queryClient.removeQueries({ queryKey: ['me'] })
    void navigate({ to: '/login', search: { avis: 'session-fermee' } })
  }, [navigate, queryClient])
}

export function useActivity(idleTimeoutSeconds: number | null | undefined) {
  const closed = useSessionClosed()
  const lastReport = useRef(Number.NEGATIVE_INFINITY)
  useEffect(() => {
    if (idleTimeoutSeconds === undefined) {
      return
    }
    let active = true
    let idle: ReturnType<typeof setTimeout> | undefined
    const arm = () => {
      clearTimeout(idle)
      if (idleTimeoutSeconds !== null) {
        idle = setTimeout(closed, idleTimeoutSeconds * 1000)
      }
    }
    const gesture = () => {
      arm()
      if (Date.now() - lastReport.current < activityEvery) {
        return
      }
      lastReport.current = Date.now()
      void api.POST('/auth/activity', { headers: json }).then(({ response }) => {
        if (active && response.status === 401) {
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
