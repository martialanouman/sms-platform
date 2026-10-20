import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect } from 'react'
import { api, json, send } from './api'

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
  return useCallback(() => {
    void navigate({ to: '/login', search: { avis: 'session-fermee' } })
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
