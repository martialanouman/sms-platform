import { permissionLabel } from './permissions'
import { Choice, Choices } from './ui'

export function PermissionPicker({
  keys,
  chosen,
  onChange,
}: {
  keys: string[]
  chosen: string[]
  onChange: (next: string[]) => void
}) {
  const resources = [...new Set(keys.map((key) => key.split(':')[0] ?? key))]
  return (
    <Choices value={chosen} onChange={onChange} className="permission-picker">
      {resources.map((resource) => (
        <fieldset className="role-choice" key={resource}>
          <legend>{permissionLabel(`${resource}:`).resource ?? resource}</legend>
          {keys
            .filter((key) => key.startsWith(`${resource}:`))
            .map((key) => (
              <Choice value={key} key={key}>
                {permissionLabel(key).action} <span className="role-keys__key">{key}</span>
              </Choice>
            ))}
        </fieldset>
      ))}
    </Choices>
  )
}
