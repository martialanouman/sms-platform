import { permissionLabel, resourceOrder } from './permissions'
import { sensitive } from './scopes.gen'
import { Choice, Choices } from './ui'

const stream = 'stream:read'
const traffic = 'traffic:read'

const rank = (resource: string) => {
  const i = resourceOrder.indexOf(resource)
  return i === -1 ? resourceOrder.length : i
}

const verbRank = (key: string) => ['read', 'write'].indexOf(key.split(':')[1] ?? '') >>> 0

export function PermissionPicker({
  keys,
  chosen,
  onChange,
  readOnly,
  streamFollowsTraffic,
}: {
  keys: string[]
  chosen: string[]
  onChange: (next: string[]) => void
  readOnly?: boolean
  streamFollowsTraffic?: boolean
}) {
  const resources = [...new Set(keys.map((key) => key.split(':')[0] ?? key))].sort(
    (a, b) => rank(a) - rank(b) || a.localeCompare(b),
  )
  const derived = (key: string) => streamFollowsTraffic && key === stream && chosen.includes(traffic)
  const derive = (next: string[]) =>
    onChange(streamFollowsTraffic && next.includes(traffic) && !next.includes(stream) ? [...next, stream] : next)
  return (
    <Choices value={chosen} onChange={derive} className="permission-picker">
      {resources.map((resource) => (
        <fieldset className="role-choice" key={resource}>
          <legend>{permissionLabel(`${resource}:`).resource ?? resource}</legend>
          {keys
            .filter((key) => key.startsWith(`${resource}:`))
            .sort((a, b) => verbRank(a) - verbRank(b) || a.localeCompare(b))
            .map((key) => (
              <Choice
                value={key}
                key={key}
                readOnly={readOnly || derived(key)}
                hint={derived(key) ? `${stream} · compris avec la lecture du trafic` : key}
              >
                {permissionLabel(key).action ?? key}
                {(sensitive as readonly string[]).includes(key) && (
                  <>
                    {' '}
                    <span className="ui-tag ui-tag--sensitive">sensible</span>
                  </>
                )}
              </Choice>
            ))}
        </fieldset>
      ))}
    </Choices>
  )
}
