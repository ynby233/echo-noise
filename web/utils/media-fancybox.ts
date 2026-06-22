import { animateFancyboxHtml5VideoClose } from './fancybox-video-close'

type MediaFancyboxOptionsConfig = {
  startIndex?: number
  withVideoClose?: boolean
  on?: Record<string, any>
}

export const createMediaFancyboxOptions = (config: MediaFancyboxOptionsConfig = {}) => {
  const options: Record<string, any> = {
    animated: true,
    closeButton: false,
    mainClass: 'noise-media-fancybox',
    Carousel: { infinite: true },
    Toolbar: {
      enabled: true,
      display: {
        left: ['infobar'],
        middle: [],
        right: ['iterateZoom', 'slideshow', 'fullscreen', 'thumbs', 'close']
      }
    },
    Images: { zoom: true },
    Html: { videoAutoplay: false },
    Thumbs: { type: 'classic', autoStart: true },
    compact: false,
    placeFocusBack: false
  }

  if (typeof config.startIndex === 'number') {
    options.startIndex = config.startIndex
  }

  if (config.withVideoClose) {
    options.on = {
      shouldClose: animateFancyboxHtml5VideoClose,
      close: animateFancyboxHtml5VideoClose,
      ...(config.on || {})
    }
  } else if (config.on) {
    options.on = config.on
  }

  return options
}
