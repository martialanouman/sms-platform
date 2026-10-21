import { useQuery, useQueryClient } from '@tanstack/react-query'
import { QRCodeSVG } from 'qrcode.react'
import { type FormEvent, useEffect, useId, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { api, json, type send, unreachable } from './api'
import type { components } from './api.gen'
import { assertPasskey, passkeysSupported, registerPasskey } from './passkey'
import { Field, useSubmission } from './ui'

type Method = 'totp' | 'recovery_code'
type Call<T = unknown> = ReturnType<Parameters<typeof send<T>>[0]>

const methods = {
  totp: {
    label: 'Code à six chiffres',
    input: { autoComplete: 'one-time-code', inputMode: 'numeric' },
    other: 'recovery_code',
    switchTo: 'Utiliser un code de récupération',
  },
  recovery_code: {
    label: 'Code de récupération',
    input: { autoComplete: 'off', autoCapitalize: 'characters', spellCheck: false },
    other: 'totp',
    switchTo: "Utiliser l'application",
  },
} as const

export function FactorProof<T>({
  action,
  prove,
  onProved,
}: {
  action: string
  prove: (method: Method, code: string) => Call<T>
  onProved: (data: T) => void
}) {
  const [method, setMethod] = useState<Method>('totp')
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()
  const field = useRef<HTMLInputElement>(null)
  const { label, input, other, switchTo } = methods[method]

  useEffect(() => {
    field.current?.focus()
  }, [])

  function switchMethod() {
    flushSync(() => {
      fail('')
      setMethod(other)
    })
    field.current?.focus()
  }

  async function verify(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const code = String(new FormData(event.currentTarget).get('code') ?? '')
    const outcome = await submit(() => prove(method, code))
    if (outcome.failure === undefined) {
      onProved(outcome.data)
    }
  }

  return (
    <form className="auth__form" onSubmit={verify}>
      <Field key={method} label={label} ref={field} name="code" required aria-describedby={describedBy} {...input} />
      {refusal}
      <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
        {action}
      </button>
      <button className="ui-button ui-button--secondary" type="button" onClick={switchMethod}>
        {switchTo}
      </button>
    </form>
  )
}

export function TotpEnrollment({
  enrollment,
  confirm,
  onDone,
  heading = 'h2',
}: {
  enrollment: components['schemas']['TotpEnrollment']
  confirm: (code: string) => Call
  onDone: () => void
  heading?: 'h2' | 'h3'
}) {
  const [confirmed, setConfirmed] = useState(false)
  const { submit, waiting, refusal, describedBy } = useSubmission()
  const intro = useRef<HTMLParagraphElement>(null)

  useEffect(() => {
    intro.current?.focus()
  }, [])

  async function verify(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const code = String(new FormData(event.currentTarget).get('code') ?? '')
    const outcome = await submit(() => confirm(code))
    if (outcome.failure === undefined) {
      setConfirmed(true)
    }
  }

  if (confirmed) {
    return <RecoveryCodes codes={enrollment.recoveryCodes} onDone={onDone} Heading={heading} />
  }
  return (
    <>
      <p ref={intro} className="auth__intro" tabIndex={-1}>
        Scannez ce QR code avec votre application d'authentification, puis saisissez le code qu'elle affiche.
      </p>
      <div className="auth__qr">
        <QRCodeSVG
          value={enrollment.otpauthUri}
          size={200}
          marginSize={4}
          title="QR code de configuration de l'application"
        />
      </div>
      <p className="auth__aside">Sans caméra, saisissez cette clé dans l'application :</p>
      <p className="auth__secret">{enrollment.secret}</p>
      <CopyButton label="Copier la clé" value={enrollment.secret} />
      <form className="auth__form" onSubmit={verify}>
        <Field
          label="Code à six chiffres"
          name="code"
          autoComplete="one-time-code"
          inputMode="numeric"
          required
          aria-describedby={describedBy}
        />
        {refusal}
        <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
          Vérifier
        </button>
      </form>
    </>
  )
}

function RecoveryCodes({ codes, onDone, Heading }: { codes: string[]; onDone: () => void; Heading: 'h2' | 'h3' }) {
  const id = useId()
  const heading = useRef<HTMLHeadingElement>(null)

  useEffect(() => {
    heading.current?.focus()
  }, [])

  return (
    <>
      <Heading ref={heading} className="auth__subtitle" id={id} tabIndex={-1}>
        Codes de récupération
      </Heading>
      <p className="auth__aside">
        Chacun ne sert qu'une fois, si l'application vous manque. Conservez-les hors de cet appareil : ils ne seront
        plus affichés.
      </p>
      <ul className="auth__codes" aria-labelledby={id}>
        {codes.map((code) => (
          <li key={code}>{code}</li>
        ))}
      </ul>
      <CopyButton label="Copier les codes" value={codes.join('\n')} />
      <button className="ui-button ui-button--primary" type="button" onClick={onDone}>
        J'ai enregistré ces codes
      </button>
    </>
  )
}

export function CopyButton({ label, value }: { label: string; value: string }) {
  const [outcome, setOutcome] = useState('')

  function copy() {
    setOutcome('')
    navigator.clipboard.writeText(value).then(
      () => setOutcome('Copié.'),
      () => setOutcome('Le navigateur a refusé la copie ; sélectionnez le texte et copiez-le.'),
    )
  }

  return (
    <>
      <button className="ui-button ui-button--secondary" type="button" onClick={copy}>
        {label}
      </button>
      <p className="auth__aside" role="status">
        {outcome}
      </p>
    </>
  )
}

export function TotpReplacement({
  held,
  remaining,
  onReplaced,
}: {
  held: boolean
  remaining: number
  onReplaced: () => void
}) {
  const id = useId()
  const adding = useSubmission()
  const [step, setStep] = useState<'idle' | 'proof' | components['schemas']['TotpEnrollment']>('idle')
  const heading = useRef<HTMLHeadingElement>(null)

  async function add() {
    const outcome = await adding.submit(() => api.POST('/auth/mfa/totp/enroll', { body: {} }))
    if (outcome.failure === undefined) {
      setStep(outcome.data)
    }
  }

  function back() {
    flushSync(() => setStep('idle'))
    heading.current?.focus()
  }

  return (
    <section className="auth__form" aria-labelledby={id}>
      <h2 ref={heading} className="auth__subtitle" id={id} tabIndex={-1}>
        Application d'authentification
      </h2>
      {step === 'idle' && !held && (
        <>
          <p className="auth__aside">Aucune application d'authentification.</p>
          {adding.refusal}
          <button className="ui-button ui-button--secondary" type="button" disabled={adding.waiting} onClick={add}>
            Ajouter l'application
          </button>
        </>
      )}
      {step === 'idle' && held && (
        <>
          <p className="auth__aside">{remaining} codes de récupération restants.</p>
          <button className="ui-button ui-button--secondary" type="button" onClick={() => setStep('proof')}>
            Remplacer l'application
          </button>
        </>
      )}
      {step === 'proof' && (
        <>
          <p className="auth__intro">Prouvez d'abord l'application actuelle, ou présentez un code de récupération.</p>
          <FactorProof
            action="Continuer"
            prove={(method, code) => api.POST('/auth/mfa/totp/enroll', { body: { method, code } })}
            onProved={setStep}
          />
          <button className="ui-button ui-button--secondary" type="button" onClick={back}>
            Annuler
          </button>
        </>
      )}
      {typeof step === 'object' && (
        <TotpEnrollment
          enrollment={step}
          confirm={(code) => api.POST('/auth/mfa/totp/confirm', { body: { code } })}
          heading="h3"
          onDone={() => {
            back()
            onReplaced()
          }}
        />
      )}
    </section>
  )
}

export function PasskeyRegistration({ action, onRegistered }: { action: string; onRegistered: () => void }) {
  const { submit, waiting, refusal, fail, describedBy } = useSubmission()

  async function register(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const name = String(new FormData(form).get('name') ?? '').trim()
    if (!name) {
      fail('Nommez la clé : ce nom la distinguera des autres.')
      return
    }
    const outcome = await submit(() => registerPasskey(name))
    if (outcome.failure === undefined) {
      form.reset()
      onRegistered()
    }
  }

  if (!passkeysSupported()) {
    return <p className="auth__aside">Ce navigateur ne sait pas créer de clé d'accès.</p>
  }
  return (
    <form className="auth__form" onSubmit={register}>
      <Field label="Nom de la clé" name="name" maxLength={64} required aria-describedby={describedBy} />
      {refusal}
      <button className="ui-button ui-button--primary" type="submit" disabled={waiting}>
        {action}
      </button>
    </form>
  )
}

export function PasskeyAssertion({ challenge, onProved }: { challenge: string; onProved: () => void }) {
  const { submit, waiting, refusal } = useSubmission()

  async function present() {
    const outcome = await submit(() => assertPasskey(challenge))
    if (outcome.failure === undefined) {
      onProved()
    }
  }

  if (!passkeysSupported()) {
    return <p className="auth__aside">Ce navigateur ne sait pas présenter de clé d'accès ; utilisez-en un à jour.</p>
  }
  return (
    <>
      {refusal}
      <button className="ui-button ui-button--primary" type="button" disabled={waiting} onClick={present}>
        Utiliser une clé d'accès
      </button>
    </>
  )
}

export function PasskeySection({ count, onChange }: { count: number; onChange: () => void }) {
  const id = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const queryClient = useQueryClient()
  const removal = useSubmission()
  const keys = useQuery({
    queryKey: ['passkeys'],
    enabled: count > 0,
    queryFn: async () => {
      const { data } = await api.GET('/auth/mfa/webauthn/passkeys')
      if (!data) {
        throw new Error('passkeys')
      }
      return data
    },
  })

  function refresh() {
    void queryClient.invalidateQueries({ queryKey: ['passkeys'] })
    onChange()
  }

  async function remove(passkeyId: string) {
    const outcome = await removal.submit(() =>
      api.DELETE('/auth/mfa/webauthn/passkeys/{passkeyId}', { params: { path: { passkeyId } }, headers: json }),
    )
    if (outcome.failure === undefined) {
      refresh()
      heading.current?.focus()
    }
  }

  return (
    <section className="auth__form" aria-labelledby={id}>
      <h2 ref={heading} className="auth__subtitle" id={id} tabIndex={-1}>
        Clés d'accès
      </h2>
      {keys.error && (
        <p className="auth__refusal" role="alert">
          {unreachable}
        </p>
      )}
      {count === 0 ? (
        <p className="auth__aside">Aucune clé d'accès sur ce compte.</p>
      ) : (
        <ul className="auth__keys">
          {keys.data?.map((key) => (
            <li key={key.id}>
              <span>{key.name}</span>
              <button
                className="ui-button ui-button--secondary"
                type="button"
                aria-label={`Retirer ${key.name}`}
                disabled={removal.waiting}
                onClick={() => remove(key.id)}
              >
                Retirer
              </button>
            </li>
          ))}
        </ul>
      )}
      {removal.refusal}
      <PasskeyRegistration action="Ajouter une clé d'accès" onRegistered={refresh} />
    </section>
  )
}
