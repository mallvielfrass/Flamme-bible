import { placeTooltip } from './tooltip.js'
import './tooltip.css'

const GAP = 6
const hosts = new WeakMap()
let bubble
let active

function ensureBubble() {
  if (bubble) return bubble
  bubble = document.createElement('div')
  bubble.className = 'icon-tip'
  bubble.setAttribute('aria-hidden', 'true')
  document.body.appendChild(bubble)
  return bubble
}

function hide() {
  if (bubble) bubble.style.visibility = 'hidden'
  active = null
}

function place(el) {
  if (!bubble || active !== el) return
  const pos = placeTooltip(
    el.getBoundingClientRect(),
    bubble.getBoundingClientRect(),
    { width: window.innerWidth, height: window.innerHeight },
    GAP,
  )
  bubble.style.top = `${pos.top}px`
  bubble.style.left = `${pos.left}px`
  bubble.style.visibility = 'visible'
}

function show(el, text, pointerType) {
  if (pointerType === 'touch' || !text) return
  const node = ensureBubble()
  node.textContent = text
  node.style.visibility = 'hidden'
  node.style.top = '0px'
  node.style.left = '0px'
  active = el
  requestAnimationFrame(() => {
    if (active === el) place(el)
  })
}

function onScrollOrResize() {
  if (active) place(active)
}

/** Директива v-tip: подсказка на body, не обрезается предками. */
export const vTip = {
  mounted(el, binding) {
    const state = { text: binding.value || '' }
    const onEnter = (event) => show(el, state.text, event.pointerType)
    const onLeave = () => {
      if (active === el) hide()
    }
    const onFocus = (event) => {
      if (event.currentTarget.matches(':focus-visible')) show(el, state.text)
    }
    el.addEventListener('pointerenter', onEnter)
    el.addEventListener('pointerleave', onLeave)
    el.addEventListener('focus', onFocus)
    el.addEventListener('blur', onLeave)
    window.addEventListener('scroll', onScrollOrResize, true)
    window.addEventListener('resize', onScrollOrResize)
    hosts.set(el, { state, onEnter, onLeave, onFocus })
  },
  updated(el, binding) {
    const rec = hosts.get(el)
    if (!rec) return
    rec.state.text = binding.value || ''
    if (active === el) {
      if (!rec.state.text) hide()
      else {
        ensureBubble().textContent = rec.state.text
        place(el)
      }
    }
  },
  unmounted(el) {
    const rec = hosts.get(el)
    if (!rec) return
    el.removeEventListener('pointerenter', rec.onEnter)
    el.removeEventListener('pointerleave', rec.onLeave)
    el.removeEventListener('focus', rec.onFocus)
    el.removeEventListener('blur', rec.onLeave)
    window.removeEventListener('scroll', onScrollOrResize, true)
    window.removeEventListener('resize', onScrollOrResize)
    if (active === el) hide()
    hosts.delete(el)
  },
}
