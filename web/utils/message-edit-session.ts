export type MessageEditSessionToken = Readonly<{
  generation: number
  messageId: number
}>

export type MessageEditSession = {
  open: (messageId: number) => MessageEditSessionToken
  close: () => void
  capture: () => MessageEditSessionToken | null
  isCurrent: (token: MessageEditSessionToken | null | undefined) => boolean
}

// Owns the identity boundary for asynchronous edit work. Upload callers must
// capture before awaiting and verify through this object before changing a draft.
export const createMessageEditSession = (): MessageEditSession => {
  let generation = 0
  let messageId = 0
  let active = false

  const token = (): MessageEditSessionToken => ({ generation, messageId })
  const open = (nextMessageId: number) => {
    generation += 1
    messageId = Number(nextMessageId || 0)
    active = messageId > 0
    return token()
  }
  const close = () => {
    generation += 1
    active = false
  }
  const capture = () => active ? token() : null
  const isCurrent = (candidate: MessageEditSessionToken | null | undefined) => Boolean(
    candidate
    && active
    && candidate.generation === generation
    && candidate.messageId === messageId,
  )

  return { open, close, capture, isCurrent }
}

export const applyCurrentEditOperation = async <T>(
  session: MessageEditSession,
  token: MessageEditSessionToken,
  operation: () => Promise<T>,
  apply: (value: T) => void | Promise<void>,
) => {
  const value = await operation()
  if (!session.isCurrent(token)) return false
  await apply(value)
  return true
}
