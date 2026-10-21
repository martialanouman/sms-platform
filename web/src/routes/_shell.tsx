import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useEffect, useId, useState } from 'react'
import { api, json, mfaRequired, send, unreachable } from '../api'
import { Unauthenticated, useActivity, useMe, useSessionClosed } from '../session'

export const Route = createFileRoute('/_shell')({ component: Shell })

const entries = [
  { to: '/', label: 'Mon compte', permission: undefined },
  { to: '/operators', label: 'Opérateurs', permission: 'operators:read' },
] as const

function Entry({ to, label, permission, held }: (typeof entries)[number] & { held: string[] }) {
  const hint = useId()
  if (permission && !held.includes(permission)) {
    return (
      <li>
        <span className="shell__link" aria-disabled="true" aria-describedby={hint}>
          {label}
        </span>
        <span className="shell__hint" id={hint}>
          Il faut la permission {permission}.
        </span>
      </li>
    )
  }
  return (
    <li>
      <Link className="shell__link" to={to} activeOptions={{ exact: true }}>
        {label}
      </Link>
    </li>
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
    <div className="shell">
      <header className="shell__header">
        <p className="shell__brand">SMS Gateway</p>
        {me.data && (
          <div className="shell__identity">
            <span>{me.data.operator.displayName}</span>
            <button className="ui-button ui-button--secondary" type="button" onClick={logout}>
              Se déconnecter
            </button>
          </div>
        )}
      </header>
      {me.data && (
        <nav className="shell__rail" aria-label="Navigation principale">
          <ul>
            {entries.map((e) => (
              <Entry key={e.to} {...e} held={me.data.permissions} />
            ))}
          </ul>
        </nav>
      )}
      <main className="shell__main">
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
            <button className="ui-button ui-button--secondary" type="button" onClick={() => void me.refetch()}>
              Réessayer
            </button>
          </div>
        )}
        {me.data && <Outlet />}
      </main>
    </div>
  )
}
