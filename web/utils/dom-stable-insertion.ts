export const withStableInsertionPoint = (
  parent: Node,
  reference: Node,
  mutate: (insertionPoint: Node) => void,
): boolean => {
  if (reference.parentNode !== parent) return false
  const ownerDocument = reference.ownerDocument ?? parent.ownerDocument
  if (!ownerDocument) return false

  const marker = ownerDocument.createComment('stable-insertion-point')
  parent.insertBefore(marker, reference)
  try {
    mutate(marker)
  } finally {
    if (marker.parentNode) marker.parentNode.removeChild(marker)
  }
  return true
}
