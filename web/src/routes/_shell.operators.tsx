import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/_shell/operators')({ component: Operators })

function Operators() {
  return (
    <section className="page">
      <h1 className="page__title">Opérateurs</h1>
    </section>
  )
}
