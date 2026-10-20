import { type ComponentProps, type ReactNode, useId } from 'react'

export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  return (
    <main className="auth">
      <div className="auth__card">
        <p className="auth__brand">SMS Gateway</p>
        <h1 className="auth__title">{title}</h1>
        {children}
      </div>
    </main>
  )
}

export function Field({ label, ...input }: { label: string } & ComponentProps<'input'>) {
  const id = useId()
  return (
    <div className="ui-field">
      <label className="ui-field__label" htmlFor={id}>
        {label}
      </label>
      <input id={id} className="ui-input" {...input} />
    </div>
  )
}
