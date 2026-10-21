import { type QueryKey, useQueryClient } from '@tanstack/react-query'
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

function invalidity(validity: ValidityState) {
  if (validity.valueMissing) {
    return 'Ce champ est obligatoire.'
  }
  return validity.typeMismatch ? 'Adresse e-mail invalide.' : 'Valeur invalide.'
}

export function Field({ label, ...input }: { label: string } & ComponentProps<'input'>) {
  const id = useId()
  const [error, setError] = useState('')
  return (
    <div className="ui-field">
      <label className="ui-field__label" htmlFor={id}>
        {label}
      </label>
      <input
        id={id}
        className="ui-input"
        {...input}
        aria-invalid={error ? true : undefined}
        aria-describedby={[error && `${id}-error`, input['aria-describedby']].filter(Boolean).join(' ') || undefined}
        onInvalid={(e) => {
          e.preventDefault()
          setError(invalidity(e.currentTarget.validity))
          if (e.currentTarget.form?.querySelector(':is(input, select, textarea):invalid') === e.currentTarget) {
            e.currentTarget.focus()
          }
        }}
        onInput={(e) => {
          if (error) {
            setError(e.currentTarget.validity.valid ? '' : invalidity(e.currentTarget.validity))
          }
        }}
      />
      {error && (
        <p className="ui-field__error" id={`${id}-error`} role="alert">
          <Icon name="bang" />
          {error}
        </p>
      )}
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

export function EmptyState({
  title,
  as: Title = 'p',
  children,
}: {
  title: string
  as?: 'h1' | 'p'
  children?: ReactNode
}) {
  return (
    <div className="ui-empty">
      <span className="ui-empty__icon">
        <Icon name="diamond" />
      </span>
      <Title className="ui-empty__title">{title}</Title>
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
  // debt: #160
  blockedReason?: string
} & Omit<ComponentProps<'button'>, 'className'>) {
  const reason = useId()
  const [dismissed, setDismissed] = useState(false)
  const [shown, setShown] = useState(false)
  useEffect(() => {
    if (!shown || dismissed) {
      return
    }
    const dismiss = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        setDismissed(true)
      }
    }
    window.addEventListener('keydown', dismiss)
    return () => window.removeEventListener('keydown', dismiss)
  }, [shown, dismissed])
  const inert = loading || blockedReason !== undefined
  const button = (
    <button
      {...rest}
      type={type}
      className={`ui-button ui-button--${variant}${size ? ` ui-button--${size}` : ''}`}
      aria-busy={loading || undefined}
      aria-disabled={inert || undefined}
      aria-describedby={blockedReason === undefined ? undefined : reason}
      onClick={inert ? (event) => event.preventDefault() : onClick}
      onFocus={() => setShown(blockedReason !== undefined)}
      onMouseEnter={() => setShown(blockedReason !== undefined)}
      onBlur={() => {
        setShown(false)
        setDismissed(false)
      }}
      onMouseLeave={() => {
        setShown(false)
        setDismissed(false)
      }}
    >
      {loading && <span className="ui-button__spinner" aria-hidden="true" />}
      {children}
    </button>
  )
  if (blockedReason === undefined) {
    return button
  }
  return (
    <span className={`ui-blocked${dismissed ? ' ui-blocked--dismissed' : ''}`}>
      {button}
      <span className="ui-tooltip" id={reason}>
        {blockedReason}
      </span>
    </span>
  )
}

export function ErrorState({
  title,
  as: Title = 'p',
  children,
  onRetry,
}: {
  title: string
  as?: 'h1' | 'h2' | 'p'
  children: ReactNode
  onRetry: () => void
}) {
  return (
    <div className="ui-error" role="alert">
      <span className="ui-error__icon">
        <Icon name="bang" />
      </span>
      <Title className="ui-error__title">{title}</Title>
      <div className="ui-error__text">{children}</div>
      <Button size="sm" onClick={onRetry}>
        Réessayer
      </Button>
    </div>
  )
}

export function LoadingState({ label, rows, line }: { label: string; rows: number; line?: boolean }) {
  return (
    <div className="ui-loading" aria-busy="true" aria-live="polite">
      <span className="ui-loading__label">{label}</span>
      {Array.from({ length: rows }, (_, i) => (
        <span className={line ? 'ui-skeleton ui-skeleton--line' : 'ui-skeleton'} key={i} />
      ))}
    </div>
  )
}

export function Toolbar({ children }: { children: ReactNode }) {
  return <div className="ui-toolbar">{children}</div>
}

export function DataTable<Row>({
  caption,
  columns,
  rows,
  rowKey,
}: {
  caption: string
  columns: { header: string; cell: (row: Row) => ReactNode; end?: boolean }[]
  rows: Row[]
  rowKey: (row: Row) => string
}) {
  return (
    <table className="ui-table">
      <caption className="ui-table__caption">{caption}</caption>
      <thead>
        <tr>
          {columns.map((c) => (
            <th
              className={c.end ? 'ui-table__header ui-table__cell--end' : 'ui-table__header'}
              key={c.header}
              scope="col"
            >
              {c.header}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr className="ui-table__row" key={rowKey(row)}>
            {columns.map((c, i) =>
              i === 0 ? (
                <th className="ui-table__cell" key={c.header} scope="row">
                  {c.cell(row)}
                </th>
              ) : (
                <td className={c.end ? 'ui-table__cell ui-table__cell--end' : 'ui-table__cell'} key={c.header}>
                  {c.cell(row)}
                </td>
              ),
            )}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function Modal({
  title,
  wide,
  onClose,
  footer,
  children,
}: {
  title: string
  wide?: boolean
  onClose: () => void
  footer: ReactNode
  children: ReactNode
}) {
  const dialog = useRef<HTMLDialogElement>(null)
  const heading = useId()
  const [opener] = useState(() => document.activeElement as HTMLElement | null)
  const pressed = useRef<EventTarget>(null)

  useEffect(() => {
    if (!dialog.current?.open) {
      dialog.current?.showModal()
    }
    return () => opener?.focus()
  }, [opener])

  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: le clic ne vise que le voile ; au clavier, Échap ferme nativement.
    <dialog
      ref={dialog}
      className={wide ? 'ui-modal ui-modal--wide' : 'ui-modal'}
      aria-labelledby={heading}
      onClose={onClose}
      onMouseDown={(event) => {
        pressed.current = event.target
      }}
      onClick={(event) => pressed.current === event.currentTarget && dialog.current?.close()}
    >
      <header className="ui-modal__head">
        <h2 className="ui-modal__title" id={heading}>
          {title}
        </h2>
        <button className="ui-modal__close" type="button" aria-label="Fermer" onClick={onClose}>
          <Icon name="times" />
        </button>
      </header>
      <div className="ui-modal__body">{children}</div>
      <div className="ui-modal__foot">{footer}</div>
    </dialog>
  )
}

export function Confirmation({
  title,
  action,
  danger,
  call,
  invalidate,
  onClose,
  onDone = onClose,
  children,
}: {
  title: string
  action: string
  danger?: boolean
  call: Parameters<typeof send>[0]
  invalidate: QueryKey
  onClose: () => void
  onDone?: () => void
  children: ReactNode
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal } = useSubmission()

  async function confirm() {
    const outcome = await submit(call)
    if (answered(outcome.response)) {
      void queryClient.invalidateQueries({ queryKey: invalidate })
    }
    if (outcome.failure === undefined) {
      onDone()
    }
  }

  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        <>
          <Button onClick={onClose}>Annuler</Button>
          <Button variant={danger ? 'danger' : 'primary'} loading={waiting} onClick={() => void confirm()}>
            {action}
          </Button>
        </>
      }
    >
      {refusal}
      {children}
      <p>L’action est enregistrée dans le journal d’audit.</p>
    </Modal>
  )
}

export const answered = (response?: Response) => !!response && response.status < 500 && response.status !== 429
