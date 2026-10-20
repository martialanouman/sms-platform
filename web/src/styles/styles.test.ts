import { expect, test } from 'vitest'

const sheets = import.meta.glob<string>('./**/*.css', { query: '?raw', import: 'default', eager: true })
const all = Object.keys(sheets)
const tokens = all.filter((f) => f.startsWith('./tokens/'))
const read = (f: string) => sheets[f] ?? ''
const declared = (files: string[]) =>
  new Set(files.flatMap((f) => [...read(f).matchAll(/(?<![\w-])(--[\w-]+)\s*:/g)].map((m) => m[1])))

test('every custom property used is a token', () => {
  const known = declared(tokens)
  const used = all.flatMap((f) => [...read(f).matchAll(/var\((--[\w-]+)/g)].map((m) => m[1]))
  expect(used.length).toBeGreaterThan(0)
  expect(used.filter((v) => !known.has(v))).toEqual([])
})

test('no token is declared outside the tokens', () => {
  expect([...declared(all.filter((f) => !tokens.includes(f)))]).toEqual([])
})

test('every weight the tokens declare is loaded', () => {
  const typography = read('./tokens/typography.css')
  const fonts = read('./tokens/fonts.css')
  const weight = Object.fromEntries([...typography.matchAll(/(--fw-\w+):\s*(\d+)/g)].map((m) => [m[1], m[2]]))
  const wanted: string[] = Object.values(weight).map((w) => `@fontsource/ibm-plex-sans/latin-${w}.css`)
  for (const [, fw] of typography.matchAll(/var\((--fw-\w+)\)[^;]*var\(--font-mono\)/g)) {
    wanted.push(`@fontsource/ibm-plex-mono/latin-${fw && weight[fw]}.css`)
  }
  const loaded = [...fonts.matchAll(/@import "([^"]+)"/g)].map((m) => m[1])
  expect(wanted.length).toBeGreaterThan(4)
  expect([...new Set(wanted)].sort()).toEqual([...loaded].sort())
})
