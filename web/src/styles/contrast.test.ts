import { expect, test } from 'vitest'

type RGBA = [number, number, number, number]

const sheets = import.meta.glob<string>('./tokens/*.css', { query: '?raw', import: 'default', eager: true })
const values = new Map(
  Object.values(sheets).flatMap((sheet) =>
    [...sheet.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)].map((m): [string, string] => [m[1] ?? '', (m[2] ?? '').trim()]),
  ),
)

function color(expr: string): RGBA {
  const ref = expr.match(/^var\((--[\w-]+)\)$/)
  if (ref) {
    const value = values.get(ref[1] ?? '')
    if (!value) throw new Error(`unknown token ${ref[1]}`)
    return color(value)
  }
  if (expr === 'transparent') return [0, 0, 0, 0]
  const hex = expr.match(/^#([0-9a-f]{6})$/i)
  if (hex) {
    const n = Number.parseInt(hex[1] ?? '', 16)
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255, 1]
  }
  const mix = expr.match(/^color-mix\(in srgb, (.+) (\d+)%, (.+)\)$/)
  if (mix) {
    const [r1, g1, b1, a1] = color(mix[1] ?? '')
    const [r2, g2, b2, a2] = color(mix[3] ?? '')
    const p = Number(mix[2]) / 100
    const alpha = a1 * p + a2 * (1 - p)
    const channel = (one: number, two: number) => (one * a1 * p + two * a2 * (1 - p)) / alpha
    return [channel(r1, r2), channel(g1, g2), channel(b1, b2), alpha]
  }
  throw new Error(`not a colour: ${expr}`)
}

function over([r1, g1, b1, a]: RGBA, [r2, g2, b2]: RGBA): RGBA {
  return [r1 * a + r2 * (1 - a), g1 * a + g2 * (1 - a), b1 * a + b2 * (1 - a), 1]
}

function linear(v: number) {
  return v / 255 <= 0.04045 ? v / 255 / 12.92 : ((v / 255 + 0.055) / 1.055) ** 2.4
}

function luminance([r, g, b]: RGBA) {
  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b)
}

function ratio(text: string, ...layers: string[]) {
  const ground = layers.map((l) => color(`var(${l})`)).reduceRight((below, top) => over(top, below))
  const [front, back] = [luminance(over(color(`var(${text})`), ground)), luminance(ground)]
  return (Math.max(front, back) + 0.05) / (Math.min(front, back) + 0.05)
}

test.each([
  ['--text-primary', '--surface-page'],
  ['--text-primary', '--surface-card'],
  ['--text-primary', '--surface-sunken'],
  ['--text-muted', '--surface-card'],
  ['--text-accent', '--surface-card'],
  ['--text-link', '--surface-card'],
  ['--action-primary-fg', '--action-primary-bg', '--surface-card'],
  ['--action-secondary-fg', '--action-secondary-bg', '--surface-page'],
  ['--text-danger-on-tint', '--tint-red', '--surface-card'],
  ['--text-success', '--tint-green', '--surface-card'],
])('%s on %s reaches WCAG AA', (text, ...layers) => {
  expect(ratio(text, ...layers)).toBeGreaterThanOrEqual(4.5)
})

test('the contrast computation matches a known pair', () => {
  values.set('--probe-black', '#000000')
  values.set('--probe-white', '#ffffff')
  expect(ratio('--probe-white', '--probe-black')).toBeCloseTo(21, 5)
})
