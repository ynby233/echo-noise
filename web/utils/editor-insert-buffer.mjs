// @ts-check

export const createPreReadyEditorInsertBuffer = () => {
  /** @type {string[]} */
  const pendingValues = []

  return {
    /** @param {string} value */
    push(value) {
      if (value) pendingValues.push(value)
    },
    /** @param {(value: string) => void} insert */
    drain(insert) {
      const values = pendingValues.splice(0, pendingValues.length)
      values.forEach(insert)
      return values.length
    },
    clear() {
      pendingValues.length = 0
    },
    size() {
      return pendingValues.length
    },
  }
}
