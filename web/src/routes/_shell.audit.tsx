import { createFileRoute, Link, Outlet } from '@tanstack/react-router'
import { useShellMe } from '../session'
import { Forbidden, PageHeader } from '../ui'

export const Route = createFileRoute('/_shell/audit')({ component: Audit })

function Audit() {
  const me = useShellMe()
  const readable = (me.data?.permissions ?? []).includes('audit:read')
  return (
    <>
      <PageHeader crumbs={['Journal d’audit']} title="Journal d’audit" />
      <section className="page">
        {readable ? (
          <>
            <nav className="audit-tabs" aria-label="Journaux d’audit">
              <Link className="audit-tabs__tab" to="/audit/control-plane">
                Administration de la passerelle
              </Link>
              <Link className="audit-tabs__tab" to="/audit/access">
                Accès au dashboard
              </Link>
            </nav>
            <Outlet />
          </>
        ) : (
          <Forbidden
            title="Accès aux journaux d’audit non accordé"
            about="Ces journaux retracent les appels à l’administration de la passerelle et les accès au tableau de bord."
            permission="audit:read"
            grantedBy="Audit"
          />
        )}
      </section>
    </>
  )
}
