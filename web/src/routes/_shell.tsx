import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { api, json, mfaRequired, send, unreachable } from '../api'
import { Unauthenticated, useActivity, useMe, useSessionClosed } from '../session'
import { Button, PageHeaderProvider, PageHeaderSlot } from '../ui'

export const Route = createFileRoute('/_shell')({ component: Shell })

const groups = [
  { label: 'Administration', entries: [{ to: '/operators', label: 'Opérateurs', permission: 'operators:read' }] },
] as const

function Rail({ held }: { held: string[] }) {
  return (
    <nav className="shell__rail" aria-label="Navigation principale">
      <Link className="rail__brand" to="/" activeOptions={{ exact: true }}>
        SMS Gateway
      </Link>
      {groups.map((group) => {
        const entries = group.entries.filter((e) => held.includes(e.permission))
        return (
          entries.length > 0 && (
            <div className="rail__group" key={group.label}>
              <p className="rail__label" id={`rail-${group.label}`}>
                {group.label}
              </p>
              <ul className="rail__list" aria-labelledby={`rail-${group.label}`}>
                {entries.map((e) => (
                  <li key={e.to}>
                    <Link className="rail__item" to={e.to}>
                      {e.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          )
        )
      })}
    </nav>
  )
}

function Shell() {
  const me = useMe()
  const closed = useSessionClosed()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [leaving, setLeaving] = useState(false)
  const [failure, setFailure] = useState('')
  useActivity(leaving ? undefined : me.data?.idleTimeoutSeconds)

  useEffect(() => {
    if (me.error instanceof Unauthenticated) {
      closed()
    }
  }, [me.error, closed])
  useEffect(() => {
    const factors = me.data?.secondFactors
    const verify = () => void navigate({ to: factors?.totp || factors?.passkeys ? '/mfa' : '/enroll' })
    window.addEventListener(mfaRequired, verify)
    return () => window.removeEventListener(mfaRequired, verify)
  }, [me.data, navigate])

  async function logout() {
    setFailure('')
    setLeaving(true)
    const { failure } = await send(() => api.POST('/auth/logout', { headers: json }))
    if (failure) {
      setLeaving(false)
      setFailure(failure)
      return
    }
    queryClient.clear()
    await navigate({ to: '/login' })
  }

  return (
    <PageHeaderProvider>
      <div className="shell">
        {/* biome-ignore lint/a11y/useValidAnchor: #contenu est une vraie destination ; le fragment seul ne donne pas le focus à main. */}
        <a
          className="shell__skip"
          href="#contenu"
          onClick={(event) => {
            event.preventDefault()
            document.getElementById('contenu')?.focus()
          }}
        >
          Aller au contenu
        </a>
        {me.data ? <Rail held={me.data.permissions} /> : <div className="shell__rail" />}
        <header className="shell__topbar">
          <PageHeaderSlot />
          {me.data && (
            <div className="topbar__end">
              <Link className="topbar__operator" to="/account">
                {me.data.operator.displayName}
              </Link>
              <Button size="sm" loading={leaving} onClick={logout}>
                Se déconnecter
              </Button>
            </div>
          )}
        </header>
        <main className="shell__content" id="contenu" tabIndex={-1}>
          {failure && (
            <p className="auth__refusal" role="alert">
              {failure}
            </p>
          )}
          {me.error && !(me.error instanceof Unauthenticated) && (
            <div className="shell__failure">
              <p className="auth__refusal" role="alert">
                {unreachable}
              </p>
              <Button size="sm" onClick={() => void me.refetch()}>
                Réessayer
              </Button>
            </div>
          )}
          {me.isPending && <p className="auth__intro">Chargement de la session…</p>}
          {me.data && <Outlet />}
        </main>
      </div>
    </PageHeaderProvider>
  )
}
