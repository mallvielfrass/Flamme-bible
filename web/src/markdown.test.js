import { describe, expect, it } from 'vitest'
import { markdownTree } from '../shared/ai/markdown.js'

function types(nodes) {
  return JSON.stringify(nodes)
}

describe('markdownTree', () => {
  it('keeps https links and drops html and javascript', () => {
    const tree = markdownTree('смотри <b>жирный</b> [сайт](https://example.com/a) и [плохо](javascript:alert(1))')
    const raw = types(tree)
    expect(raw).toContain('https://example.com/a')
    expect(raw).not.toContain('"href":"javascript:')
    expect(raw).not.toContain('"type":"html"')
    expect(raw).toContain('жирный')
  })
})
