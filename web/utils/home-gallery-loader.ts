type HomeGalleryLoaderOptions = {
  load: () => Promise<any[]>
  apply: (images: any[]) => void
  clear: () => void
}

export function createHomeGalleryLoader({ load, apply, clear }: HomeGalleryLoaderOptions) {
  let generation = 0
  let inFlight: { generation: number, promise: Promise<void> } | null = null

  const request = () => {
    if (inFlight?.generation === generation) return inFlight.promise

    const requestGeneration = generation
    const promise = Promise.resolve(load())
      .then((images) => {
        if (requestGeneration === generation) apply(images)
      })
      .catch(() => {
        if (requestGeneration === generation) clear()
      })
      .finally(() => {
        if (inFlight?.generation === requestGeneration) inFlight = null
      })
    inFlight = { generation: requestGeneration, promise }
    return promise
  }

  return {
    async onConfigResolved(enabled: boolean | undefined) {
      if (enabled === false) {
        generation += 1
        clear()
        return
      }
      await request()
    },
    async onEnabledChanged(enabled: boolean | undefined) {
      if (enabled === false) {
        generation += 1
        clear()
        return
      }
      await request()
    },
    async onViewerChanged(enabled: boolean | undefined) {
      generation += 1
      clear()
      if (enabled !== false) await request()
    },
  }
}
