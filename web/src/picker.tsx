import { permissionLabel } from './permissions'

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
    <div className="permission-picker">
      {resources.map((resource) => (
        <fieldset className="role-choice" key={resource}>
          <legend>{permissionLabel(`${resource}:`).resource ?? resource}</legend>
          {keys
            .filter((key) => key.startsWith(`${resource}:`))
            .map((key) => (
              <label className="role-choice__option" key={key}>
                <input
                  type="checkbox"
                  checked={chosen.includes(key)}
                  onChange={(e) => onChange(e.target.checked ? [...chosen, key] : chosen.filter((p) => p !== key))}
                />
                {permissionLabel(key).action} <span className="role-keys__key">{key}</span>
              </label>
            ))}
        </fieldset>
      ))}
    </div>
  )
}
