import MarkdownIt from 'markdown-it'
import { splitEmoji } from '../../src/emoji.js'
import { parseResults } from './results.js'

const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})

md.validateLink = (url) => /^https?:\/\//i.test(String(url || '').trim())

/**
 * @typedef {Object} MdNode
 * @property {'text'|'emoji'|'p'|'br'|'strong'|'em'|'s'|'code'|'pre'|'a'|'ul'|'ol'|'li'|'quote'|'h'|'group'|'results'} type
 * @property {import('./results.js').AiResult[]} [items]
 * @property {string} [text]
 * @property {string} [href]
 * @property {string} [cps]
 * @property {number} [level]
 * @property {MdNode[]} [children]
 */

/**
 * Разбирает markdown в дерево. HTML из текста модели не исполняется.
 * Ссылка остаётся только если это http(s).
 * @param {string} src
 * @returns {MdNode[]}
 */
export function markdownTree(src) {
  return blocks(md.parse(String(src || ''), {}))
}

function blocks(tokens) {
  /** @type {MdNode[][]} */
  const stack = [[]]
  for (const tok of tokens) {
    if (tok.type === 'inline') {
      for (const node of inline(tok.children || [])) push(stack, node)
      continue
    }
    if (tok.type === 'fence' && String(tok.info || '').trim() === 'results') {
      const items = parseResults(tok.content || '')
      if (items.length) {
        push(stack, { type: 'results', items })
        continue
      }
    }
    if (tok.type === 'fence' || tok.type === 'code_block') {
      push(stack, { type: 'pre', text: tok.content || '' })
      continue
    }
    if (tok.type === 'html_block' || tok.type === 'html_inline') {
      push(stack, { type: 'text', text: tok.content || '' })
      continue
    }
    if (tok.nesting === 1) {
      const node = openBlock(tok)
      push(stack, node)
      stack.push(node.children)
    } else if (tok.nesting === -1 && stack.length > 1) {
      stack.pop()
    }
  }
  return stack[0]
}

function openBlock(tok) {
  if (tok.type === 'heading_open') {
    const level = Number(String(tok.tag || 'h1').slice(1)) || 1
    return { type: 'h', level, children: [] }
  }
  const type = {
    paragraph_open: 'p',
    bullet_list_open: 'ul',
    ordered_list_open: 'ol',
    list_item_open: 'li',
    blockquote_open: 'quote',
  }[tok.type] || 'group'
  return { type, children: [] }
}

function inline(tokens) {
  /** @type {MdNode[]} */
  const out = []
  /** @type {MdNode[][]} */
  const stack = [out]
  for (const tok of tokens) {
    if (tok.type === 'text') {
      for (const node of splitEmoji(tok.content || '')) push(stack, node)
      continue
    }
    if (tok.type === 'code_inline') {
      push(stack, { type: 'code', text: tok.content || '' })
      continue
    }
    if (tok.type === 'softbreak' || tok.type === 'hardbreak') {
      push(stack, { type: 'br' })
      continue
    }
    if (tok.type === 'image') {
      for (const node of inline(tok.children || [])) push(stack, node)
      continue
    }
    if (tok.type === 'html_inline') {
      push(stack, { type: 'text', text: tok.content || '' })
      continue
    }
    if (tok.nesting === 1) {
      const node = openInline(tok)
      push(stack, node)
      stack.push(node.children)
    } else if (tok.nesting === -1 && stack.length > 1) {
      stack.pop()
    }
  }
  return out
}

function openInline(tok) {
  if (tok.type === 'link_open') {
    const href = tok.attrGet('href') || ''
    if (!md.validateLink(href)) return { type: 'group', children: [] }
    return { type: 'a', href, children: [] }
  }
  const type = { strong_open: 'strong', em_open: 'em', s_open: 's' }[tok.type] || 'group'
  return { type, children: [] }
}

function push(stack, node) {
  stack[stack.length - 1].push(node)
}
