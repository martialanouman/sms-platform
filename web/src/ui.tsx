import { type ComponentProps, type ReactNode, useEffect, useId, useRef, useState } from 'react'
import { send } from './api'

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

export function useSubmission() {
  const id = useId()
  const [error, setError] = useState('')
  const [waiting, setWaiting] = useState(false)
  const alert = useRef<HTMLParagraphElement>(null)

  useEffect(() => {
    if (error) {
      alert.current?.focus()
    }
  }, [error])

  async function submit<T>(call: Parameters<typeof send<T>>[0]) {
    setError('')
    setWaiting(true)
    const outcome = await send(call)
    if (outcome.failure !== undefined) {
      setError(outcome.failure)
      const retry = Number(outcome.response?.headers.get('Retry-After'))
      if (outcome.response?.status === 429 && retry > 0) {
        setTimeout(() => setWaiting(false), retry * 1000)
        return outcome
      }
    }
    setWaiting(false)
    return outcome
  }

  const refusal = error && (
    <p ref={alert} className="auth__refusal" id={id} role="alert" tabIndex={-1}>
      {error}
    </p>
  )
  return { submit, waiting, refusal, fail: setError, describedBy: error ? id : undefined }
}
