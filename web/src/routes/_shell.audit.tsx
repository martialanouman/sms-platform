import { createFileRoute, Link, Outlet } from '@tanstack/react-router'
import { useShellMe } from '../session'
import { PageHeader } from '../ui'

export const Route = createFileRoute('/_shell/audit')({ component: Audit })

function Audit() {
  const me = useShellMe()
  const readable = (me.data?.permissions ?? []).includes('audit:read')
  return (
    <>
      <PageHeader title="Audit" />
      <section className="page">
        {readable ? (
          <>
            <nav className="audit-tabs" aria-label="Journaux d’audit">
              <Link className="audit-tabs__tab" to="/audit/control-plane">
                Plan de contrôle
              </Link>
              <Link className="audit-tabs__tab" to="/audit/access">
                Accès au dashboard
              </Link>
            </nav>
            <Outlet />
          </>
        ) : (
          <p className="auth__intro">Il faut la permission audit:read pour lire les journaux d’audit.</p>
        )}
      </section>
    </>
  )
}
