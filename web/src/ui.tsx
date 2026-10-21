import {
  type ComponentProps,
  createContext,
  type ReactNode,
  type Ref,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react'
import { createPortal } from 'react-dom'
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

const glyphs = {
  diamond: <path d="M8 2.6 13.4 8 8 13.4 2.6 8Z" />,
  bang: (
    <>
      <path d="M8 3.4v6" />
      <path d="M8 12.2h.01" />
    </>
  ),
  times: (
    <>
      <path d="M4 4l8 8" />
      <path d="M12 4l-8 8" />
    </>
  ),
}

export function Icon({ name }: { name: keyof typeof glyphs }) {
  return (
    <svg
      className="ui-icon"
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      {glyphs[name]}
    </svg>
  )
}

const HeaderSlot = createContext<[HTMLElement | null, (node: HTMLElement | null) => void]>([null, () => {}])

export function PageHeaderProvider({ children }: { children: ReactNode }) {
  const slot = useState<HTMLElement | null>(null)
  return <HeaderSlot.Provider value={slot}>{children}</HeaderSlot.Provider>
}

export function PageHeaderSlot() {
  return <div className="topbar__start" ref={useContext(HeaderSlot)[1]} />
}

export function PageHeader({ title, titleRef }: { title: string; titleRef?: Ref<HTMLHeadingElement> }) {
  const [node] = useContext(HeaderSlot)
  useEffect(() => {
    document.title = `${title} — Tableau de bord`
  }, [title])
  return (
    node &&
    createPortal(
      <h1 className="page-header__title" ref={titleRef} tabIndex={-1}>
        {title}
      </h1>,
      node,
    )
  )
}

export function EmptyState({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="ui-empty">
      <span className="ui-empty__icon">
        <Icon name="diamond" />
      </span>
      <h1 className="ui-empty__title">{title}</h1>
      {children && <div className="ui-empty__text">{children}</div>}
    </div>
  )
}

export function Button({
  variant = 'secondary',
  size,
  loading,
  blockedReason,
  type = 'button',
  onClick,
  children,
  ...rest
}: {
  variant?: 'primary' | 'secondary' | 'danger'
  size?: 'sm'
  loading?: boolean
  blockedReason?: string
} & Omit<ComponentProps<'button'>, 'className'>) {
  const reason = useId()
  const inert = loading || blockedReason !== undefined
  const button = (
    <button
      {...rest}
      type={type}
      className={`ui-button ui-button--${variant}${size ? ` ui-button--${size}` : ''}`}
      aria-busy={loading || undefined}
      aria-disabled={inert || undefined}
      aria-describedby={blockedReason === undefined ? rest['aria-describedby'] : reason}
      onClick={inert ? (event) => event.preventDefault() : onClick}
    >
      {children}
    </button>
  )
  if (blockedReason === undefined) {
    return button
  }
  return (
    <span className="ui-blocked">
      {button}
      <span className="ui-tooltip" id={reason}>
        {blockedReason}
      </span>
    </span>
  )
}
