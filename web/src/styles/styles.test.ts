import { expect, test } from 'vitest'

const sheets = import.meta.glob<string>('./**/*.css', { query: '?raw', import: 'default', eager: true })
const all = Object.keys(sheets)
const tokens = all.filter((f) => f.startsWith('./tokens/'))
const read = (f: string) => sheets[f] ?? ''
const names = (css: string) => [...css.matchAll(/(?<![\w-])(--[\w-]+)\s*:/g)].map((m) => m[1] ?? '')
const roots = (css: string) => [...css.matchAll(/:root\s*\{([^}]*)\}/g)].map((m) => m[1] ?? '').join('')
const declared = (files: string[]) => new Set(files.flatMap((f) => names(roots(read(f)))))

const positioned = ['--anchor-width']

test('every custom property used is a token or set by a Base UI positioner', () => {
  const known = new Set([...declared(tokens), ...positioned])
  const used = all.flatMap((f) => [...read(f).matchAll(/var\((--[\w-]+)/g)].map((m) => m[1] ?? ''))
  expect(used.length).toBeGreaterThan(0)
  expect(used.filter((v) => !known.has(v))).toEqual([])
})

test('a token is declared under :root of one token sheet, once', () => {
  expect(all.filter((f) => !tokens.includes(f)).flatMap((f) => names(read(f)))).toEqual([])
  expect(tokens.flatMap((f) => names(read(f).replace(/:root\s*\{[^}]*\}/g, '')))).toEqual([])
  const each = tokens.flatMap((f) => names(roots(read(f))))
  expect(each.length).toBeGreaterThan(100)
  expect(each.filter((n, i) => each.indexOf(n) !== i)).toEqual([])
})

test('the entry sheet imports every token sheet', () => {
  const imported = [...read('./app.css').matchAll(/@import "([^"]+)"/g)].map((m) => m[1])
  expect(imported.sort()).toEqual(tokens.sort())
})

test('every weight the tokens declare is loaded', () => {
  const typography = read('./tokens/typography.css')
  const fonts = read('./tokens/fonts.css')
  const weights = [...typography.matchAll(/--fw-\w+:\s*(\d+)/g)].map((m) => m[1])
  const wanted = ['sans', 'mono'].flatMap((family) =>
    weights.map((w) => `@fontsource/ibm-plex-${family}/latin-${w}.css`),
  )
  const loaded = [...fonts.matchAll(/@import "([^"]+)"/g)].map((m) => m[1])
  expect(weights.length).toBeGreaterThan(2)
  expect([...new Set(wanted)].sort()).toEqual([...loaded].sort())
})

test('a field draws its boundary with the tokens that reach 3:1', () => {
  const rule = (selector: string) => read('./app.css').match(new RegExp(`\\n${selector} \\{([^}]*)\\}`))?.[1] ?? ''
  expect(rule('\\.ui-input')).toMatch(/border: 1px solid var\(--border-field\);/)
  expect(rule('\\.ui-input:hover')).toMatch(/border-color: var\(--border-field-hover\);/)
})
