import { AlertDialog } from '@base-ui/react/alert-dialog'
import { Button as BaseButton } from '@base-ui/react/button'
import { Checkbox } from '@base-ui/react/checkbox'
import { CheckboxGroup } from '@base-ui/react/checkbox-group'
import { Combobox as BaseCombobox } from '@base-ui/react/combobox'
import { Dialog } from '@base-ui/react/dialog'
import { Field as BaseField } from '@base-ui/react/field'
import { Select as BaseSelect } from '@base-ui/react/select'
import { Tooltip } from '@base-ui/react/tooltip'
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
import { permissionLabel } from './permissions'

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
  const [error, setError] = useState('')
  return (
    <BaseField.Root className="ui-field" invalid={error !== ''}>
      <BaseField.Label className="ui-field__label">{label}</BaseField.Label>
      <BaseField.Control
        className="ui-input"
        {...input}
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
      <BaseField.Error className="ui-field__error" role="alert" match={error !== ''}>
        <Icon name="bang" />
        {error}
      </BaseField.Error>
    </BaseField.Root>
  )
}

export function Combobox<Item>({
  label,
  name,
  items,
  defaultValue,
  itemLabel,
  itemValue,
  itemDetail,
  matches,
  empty,
}: {
  label: string
  name: string
  items: Item[]
  defaultValue?: Item | null
  itemLabel: (item: Item) => string
  itemValue: (item: Item) => string
  itemDetail: (item: Item) => ReactNode
  matches: (item: Item, query: string) => boolean
  empty: string
}) {
  const id = useId()
  return (
    <BaseCombobox.Root
      items={items}
      name={name}
      defaultValue={defaultValue}
      itemToStringLabel={itemLabel}
      itemToStringValue={itemValue}
      filter={matches}
    >
      <div className="ui-field">
        <label className="ui-field__label" htmlFor={id}>
          {label}
        </label>
        <BaseCombobox.Input id={id} className="ui-input" />
      </div>
      <BaseCombobox.Portal>
        <BaseCombobox.Positioner className="ui-combobox__positioner" sideOffset={4}>
          <BaseCombobox.Popup className="ui-combobox__popup">
            <BaseCombobox.Empty className="ui-combobox__empty">{empty}</BaseCombobox.Empty>
            <BaseCombobox.List>
              {(item: Item) => (
                <BaseCombobox.Item key={itemValue(item)} value={item} className="ui-combobox__item">
                  <span>{itemLabel(item)}</span>
                  <span className="ui-combobox__detail">{itemDetail(item)}</span>
                </BaseCombobox.Item>
              )}
            </BaseCombobox.List>
          </BaseCombobox.Popup>
        </BaseCombobox.Positioner>
      </BaseCombobox.Portal>
    </BaseCombobox.Root>
  )
}

export function Select({
  label,
  items,
  value,
  onChange,
  blockedReason,
}: {
  label: string
  items: { value: string; label: string }[]
  value: string
  onChange: (value: string) => void
  blockedReason?: string
}) {
  const reason = useId()
  return (
    <BaseSelect.Root
      items={items}
      value={value}
      onValueChange={(next) => onChange(next ?? '')}
      open={blockedReason === undefined ? undefined : false}
    >
      {blockedReason !== undefined && (
        <span className="ui-visually-hidden" id={reason}>
          {blockedReason}
        </span>
      )}
      <BaseSelect.Trigger
        className="ui-input ui-select"
        aria-label={label}
        aria-describedby={blockedReason === undefined ? undefined : reason}
        aria-disabled={blockedReason !== undefined || undefined}
        title={blockedReason}
      >
        {label} : {items.find((item) => item.value === value)?.label ?? value}
        <BaseSelect.Icon className="ui-select__icon">
          <Icon name="chevron" />
        </BaseSelect.Icon>
      </BaseSelect.Trigger>
      <BaseSelect.Portal>
        <BaseSelect.Positioner className="ui-combobox__positioner" sideOffset={4} alignItemWithTrigger={false}>
          <BaseSelect.Popup className="ui-combobox__popup">
            <BaseSelect.List>
              {items.map((item) => (
                <BaseSelect.Item key={item.value} value={item.value} className="ui-combobox__item">
                  <BaseSelect.ItemText>{item.label}</BaseSelect.ItemText>
                </BaseSelect.Item>
              ))}
            </BaseSelect.List>
          </BaseSelect.Popup>
        </BaseSelect.Positioner>
      </BaseSelect.Portal>
    </BaseSelect.Root>
  )
}

export function Choices({
  value,
  onChange,
  className,
  children,
}: {
  value: string[]
  onChange: (next: string[]) => void
  className?: string
  children: ReactNode
}) {
  return (
    <CheckboxGroup value={value} onValueChange={onChange} className={className}>
      {children}
    </CheckboxGroup>
  )
}

const CheckboxRoot = Checkbox.Root

export function Choice({ value, disabled, children }: { value: string; disabled?: boolean; children: ReactNode }) {
  return (
    <label className="role-choice__option">
      <CheckboxRoot value={value} disabled={disabled} className="ui-check">
        <Checkbox.Indicator className="ui-check__mark">
          <Icon name="check" />
        </Checkbox.Indicator>
      </CheckboxRoot>
      {children}
    </label>
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
  check: <path d="M3.4 8.6 6.4 11.6 12.6 4.9" />,
  chevron: <path d="M4 6.2 8 10.2 12 6.2" />,
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
  as?: 'h1' | 'h2' | 'p'
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

export function OperatorCell({ name, email }: { name: string; email: string }) {
  return (
    <span className="operator-cell">
      <span>{name}</span>
      <span className="operator-cell__email">{email}</span>
    </span>
  )
}

export function Forbidden({
  title,
  about,
  permission,
  grantedBy,
}: {
  title: string
  about: string
  permission: string
  grantedBy: string
}) {
  const { resource, action } = permissionLabel(permission)
  return (
    <EmptyState as="h2" title={title}>
      {about} Cet écran demande la permission {resource} : {action?.toLocaleLowerCase('fr')}{' '}
      <span className="role-keys__key">{permission}</span>, qu’aucun rôle de ce compte n’accorde. Un opérateur habilité
      à modifier les rôles des opérateurs peut ajouter le rôle « {grantedBy} » à ce compte.
    </EmptyState>
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
  const inert = loading || blockedReason !== undefined
  const button = (
    <BaseButton
      {...rest}
      type={type}
      className={`ui-button ui-button--${variant}${size ? ` ui-button--${size}` : ''}`}
      aria-busy={loading || undefined}
      aria-disabled={inert || undefined}
      aria-describedby={blockedReason === undefined ? undefined : reason}
      onClick={inert ? (event) => event.preventDefault() : onClick}
    >
      {loading && <span className="ui-button__spinner" aria-hidden="true" />}
      {children}
    </BaseButton>
  )
  if (blockedReason === undefined) {
    return button
  }
  return (
    <Tooltip.Root>
      <Tooltip.Trigger delay={0} closeOnClick={false} render={button} />
      <span className="ui-visually-hidden" id={reason}>
        {blockedReason}
      </span>
      <Tooltip.Portal>
        <Tooltip.Positioner className="ui-tooltip__positioner" side="bottom" align="end" sideOffset={6}>
          <Tooltip.Popup className="ui-tooltip">{blockedReason}</Tooltip.Popup>
        </Tooltip.Positioner>
      </Tooltip.Portal>
    </Tooltip.Root>
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

export function Toolbar({ filters, children }: { filters?: ReactNode; children?: ReactNode }) {
  return (
    <div className="ui-toolbar">
      {filters && <div className="ui-toolbar__filters">{filters}</div>}
      {children}
    </div>
  )
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
  alert,
  onClose,
  footer,
  children,
}: {
  title: string
  wide?: boolean
  alert?: boolean
  onClose: () => void
  footer: ReactNode
  children: ReactNode
}) {
  const [opener] = useState(() => document.activeElement as HTMLElement | null)
  useEffect(() => () => opener?.focus(), [opener])
  const Kind = alert ? AlertDialog : Dialog
  return (
    <Kind.Root open onOpenChange={(open) => !open && onClose()}>
      <Kind.Portal>
        <Kind.Backdrop className="ui-modal__backdrop" />
        <Kind.Popup className={wide ? 'ui-modal ui-modal--wide' : 'ui-modal'}>
          <header className="ui-modal__head">
            <Kind.Title className="ui-modal__title">{title}</Kind.Title>
            <Kind.Close className="ui-modal__close" aria-label="Fermer">
              <Icon name="times" />
            </Kind.Close>
          </header>
          <div className="ui-modal__body">{children}</div>
          <div className="ui-modal__foot">{footer}</div>
        </Kind.Popup>
      </Kind.Portal>
    </Kind.Root>
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
  refusals = {},
  children,
}: {
  title: string
  action: string
  danger?: boolean
  call: Parameters<typeof send>[0]
  invalidate: QueryKey
  onClose: () => void
  onDone?: () => void
  refusals?: Record<number, string>
  children: ReactNode
}) {
  const queryClient = useQueryClient()
  const { submit, waiting, refusal, fail } = useSubmission()

  async function confirm() {
    const outcome = await submit(call)
    if (answered(outcome.response)) {
      void queryClient.invalidateQueries({ queryKey: invalidate })
    }
    const said = outcome.response && refusals[outcome.response.status]
    if (said) {
      fail(said)
    }
    if (outcome.failure === undefined) {
      onDone()
    }
  }

  return (
    <Modal
      alert
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
