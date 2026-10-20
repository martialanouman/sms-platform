import { api, json } from './api'

type Answer = Promise<{ data?: unknown; error?: unknown; response: Response }>

export const passkeysSupported = () =>
  typeof PublicKeyCredential !== 'undefined' && typeof PublicKeyCredential.parseCreationOptionsFromJSON === 'function'

const unanswered = () => ({
  error: {
    message: "La clé d'accès n'a pas répondu : la fenêtre s'est refermée, ou l'appareil a refusé. Recommencez.",
  },
  response: new Response(null, { status: 400 }),
})

export async function registerPasskey(name: string): Answer {
  const begun = await api.POST('/auth/mfa/webauthn/register/begin', { headers: json })
  if (!begun.data) {
    return begun
  }
  let attestation: Record<string, unknown>
  try {
    const options = PublicKeyCredential.parseCreationOptionsFromJSON(
      begun.data.publicKey as PublicKeyCredentialCreationOptionsJSON,
    )
    attestation = ((await navigator.credentials.create({ publicKey: options })) as PublicKeyCredential).toJSON()
  } catch {
    return unanswered()
  }
  return api.POST('/auth/mfa/webauthn/register/finish', { body: { name, attestation } })
}

export async function assertPasskey(challenge: string): Answer {
  const begun = await api.POST('/auth/mfa/webauthn/assert/begin', { headers: json })
  if (!begun.data) {
    return begun
  }
  let assertion: Record<string, unknown>
  try {
    const options = PublicKeyCredential.parseRequestOptionsFromJSON(
      begun.data.publicKey as PublicKeyCredentialRequestOptionsJSON,
    )
    assertion = ((await navigator.credentials.get({ publicKey: options })) as PublicKeyCredential).toJSON()
  } catch {
    return unanswered()
  }
  return api.POST('/auth/mfa/verify', { body: { challenge, method: 'webauthn', assertion } })
}
