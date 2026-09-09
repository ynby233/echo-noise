export const rectanglesOverlap = (
  source: Pick<DOMRect, 'left' | 'top' | 'right' | 'bottom'>,
  target: Pick<DOMRect, 'left' | 'top' | 'right' | 'bottom'>,
) => (
  source.left < target.right &&
  source.right > target.left &&
  source.top < target.bottom &&
  source.bottom > target.top
)
