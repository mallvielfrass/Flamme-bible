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

  it('turns a results fence into song and verse items', () => {
    const tree = markdownTree([
      'Смотри',
      '',
      '```results',
      '[{"kind":"song","title":"Благодать","lyrics":"Куплет"},{"kind":"verse","id":"Ин. 3:16","reference":"Ин. 3:16","text":"Ибо так"}]',
      '```',
      '',
      '```results',
      'не json',
      '```',
    ].join('\n'))
    const raw = types(tree)
    expect(raw).toContain('"type":"results"')
    expect(raw).toContain('Куплет')
    expect(raw).toContain('Ибо так')
    expect(raw).toContain('"type":"pre"')
    expect(raw).toContain('не json')
  })
})
