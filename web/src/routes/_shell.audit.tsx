import { createFileRoute, Outlet } from '@tanstack/react-router'
import { useShellMe } from '../session'
import { Forbidden, PageHeader, Tabs } from '../ui'

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
            <Tabs
              label="Journaux d’audit"
              tabs={[
                { to: '/audit/control-plane', label: 'Administration de la passerelle' },
                { to: '/audit/access', label: 'Accès au dashboard' },
              ]}
            />
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
