/** Положение подсказки для position: fixed. Сверху, если хватает места, иначе снизу. */
export function placeTooltip(trigger, tip, viewport, gap = 6) {
  let top = trigger.top - tip.height - gap
  if (top < gap) {
    top = trigger.bottom + gap
    if (top + tip.height > viewport.height - gap) {
      top = Math.max(gap, viewport.height - tip.height - gap)
    }
  }
  let left = trigger.left + trigger.width / 2 - tip.width / 2
  const maxLeft = Math.max(gap, viewport.width - tip.width - gap)
  left = Math.min(Math.max(gap, left), maxLeft)
  return { top, left }
}
