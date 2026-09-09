import { withStableInsertionPoint } from './dom-stable-insertion'

// Owns the DOM grouping and loading placeholders for rendered image/video runs.
// Callers provide the current render root after each preview replacement.
export const applyRenderedMediaLayout = (root: HTMLElement | null, keepImagesFullSize = false) => {
  if (!root) return;

  root.querySelectorAll('.image-grid, .single-media, .full-image-attachment').forEach((node) => {
    const parent = node.parentElement
    if (!parent) return
    while (node.firstChild) {
      parent.insertBefore(node.firstChild, node)
    }
    node.remove()
  })

  const isMediaNode = (node: any): boolean => {
    if (!node || node.nodeType !== Node.ELEMENT_NODE) return false;
    const el = node as Element;
    const tag = el.tagName.toLowerCase();
    return tag === 'img' ||
           tag === 'video' ||
           el.classList.contains('video-wrapper') ||
           (tag === 'a' && el.querySelector('img') !== null);
  };

  const isSingleMediaWrapper = (el: Element | null) => {
    if (!el) return false
    return el.classList.contains('single-media') && Array.from(el.children).some((child) => isMediaNode(child))
  }

  const fullSizeRenderSelector = [
    '[data-render-source="xiaohongshu"]',
    '[data-render-source="xhs"]',
    '[data-render-source="rednote"]',
    '.xiaohongshu-render',
    '.xhs-render',
    '.rednote-render',
    '.xiaohongshu-render-image',
    '.xhs-render-image',
    '.rednote-render-image',
  ].join(',')

  const shouldKeepFullSizeImage = (el: Element | null) => !!el?.closest(fullSizeRenderSelector)

  const getPlainImage = (node: HTMLElement): HTMLImageElement | null => {
    if (node.closest('.github-card')) return null
    if (shouldKeepFullSizeImage(node)) return null
    const tagName = node.tagName.toLowerCase()
    if (tagName === 'img') return node as HTMLImageElement
    if (tagName === 'a' && !node.closest('.github-card, .video-wrapper, .douyin-video-wrapper')) {
      const img = node.querySelector('img') as HTMLImageElement | null
      if (shouldKeepFullSizeImage(img)) return null
      return img
    }
    return null
  }

  const isPlainImageNode = (node: HTMLElement) => !!getPlainImage(node)

  const ensureImageAnchor = (node: HTMLElement, group: string): HTMLElement => {
    const img = getPlainImage(node)
    if (!img) return node
    const src = img.getAttribute('src') || img.currentSrc || img.src || ''
    if (node.tagName.toLowerCase() === 'a') {
      const anchor = node as HTMLAnchorElement
      const href = anchor.getAttribute('href') || ''
      if (!href || href === '#' || href.startsWith('javascript:')) anchor.setAttribute('href', src)
      anchor.setAttribute('data-fancybox', group)
      anchor.classList.add('inline-image-link')
      return anchor
    }
    const anchor = document.createElement('a')
    anchor.setAttribute('href', src)
    anchor.setAttribute('data-fancybox', group)
    anchor.className = 'inline-image-link'
    anchor.appendChild(img)
    return anchor
  }

  const isPureMediaParagraph = (p: Element) => {
    const children = Array.from(p.childNodes);
    if (children.length === 0) return false;
    let hasMedia = false;
    for (const child of children) {
      const node = child as Node;
      if (node.nodeType === Node.ELEMENT_NODE) {
        if (isMediaNode(node)) {
          hasMedia = true;
          continue;
        }
        if ((node as Element).tagName.toLowerCase() === 'br') continue;
        return false;
      } else if (node.nodeType === Node.TEXT_NODE) {
        if ((node.textContent || '').trim() !== '') return false;
      }
    }
    return hasMedia;
  };

  const areAdjacent = (a: Element, b: Element) => {
     let next = a.nextSibling;
     while (next && next !== b) {
       if (next.nodeType === Node.ELEMENT_NODE) return false;
       if (next.nodeType === Node.TEXT_NODE) {
         if ((next.textContent || '').trim() !== '') return false;
       }
       next = next.nextSibling;
     }
     return next === b;
  };

  // 1. Identify Candidates
  const allCandidates = Array.from(root.querySelectorAll('p, img, a, video, .video-wrapper, .single-media')) as HTMLElement[];
  const blocks: HTMLElement[] = [];

  for (const el of allCandidates) {
     if (el.closest('.github-card')) continue;
     const tag = el.tagName.toLowerCase();
     if (tag === 'p') {
       if (isPureMediaParagraph(el)) blocks.push(el);
     } else if (tag === 'img') {
       const parent = el.parentElement;
       if (!parent) continue;
       if (parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       if (parent.tagName.toLowerCase() === 'a') continue;
       blocks.push(el);
     } else if (tag === 'a') {
       if (!el.querySelector('img')) continue;
       const parent = el.parentElement;
       if (!parent) continue;
       if (parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       blocks.push(el);
     } else if (isSingleMediaWrapper(el)) {
       blocks.push(el)
     } else {
       const parent = el.parentElement;
       if (parent && parent.tagName.toLowerCase() === 'p' && isPureMediaParagraph(parent)) continue;
       if (tag === 'video' && parent && parent.classList.contains('video-wrapper')) continue;
       blocks.push(el);
     }
  }

  // 2. Group by Parent
  const blocksByParent = new Map<HTMLElement, HTMLElement[]>();
  for (const block of blocks) {
     const parent = block.parentElement;
     if (!parent) continue;
     if (!blocksByParent.has(parent)) blocksByParent.set(parent, []);
     blocksByParent.get(parent)!.push(block);
  }

  // 3. Process Runs
  for (const [parent, children] of blocksByParent) {
     const runs: HTMLElement[][] = [];
     let current: HTMLElement[] = [];
     for (const block of children) {
        if (current.length === 0) {
           current.push(block);
        } else {
           const last = current[current.length - 1];
           if (areAdjacent(last, block)) {
              current.push(block);
           } else {
              runs.push(current);
              current = [block];
           }
        }
     }
     if (current.length > 0) runs.push(current);

     for (const run of runs) {
        const mediaItems: { node: HTMLElement }[] = [];
        for (const block of run) {
           if (block.tagName.toLowerCase() === 'p') {
              block.childNodes.forEach((node) => {
                 if (isMediaNode(node)) mediaItems.push({ node: node as HTMLElement });
              });
           } else if (isSingleMediaWrapper(block)) {
             Array.from(block.children).forEach((node) => {
               if (isMediaNode(node)) mediaItems.push({ node: node as HTMLElement })
             })
           } else {
              mediaItems.push({ node: block });
           }
        }

        if (keepImagesFullSize && mediaItems.length > 0 && mediaItems.every(({ node }) => isPlainImageNode(node))) {
          const firstBlock = run[0]
          const parentNode = firstBlock?.parentNode
          if (parentNode) {
            const group = `full-image-${Math.random().toString(36).slice(2)}`
            const movedNodes = new Set(mediaItems.map(({ node }) => node))
            const inserted = withStableInsertionPoint(parentNode, firstBlock, (insertionPoint) => {
              for (const { node } of mediaItems) {
                const wrapper = document.createElement('div')
                wrapper.className = 'full-image-attachment'
                parentNode.insertBefore(wrapper, insertionPoint)
                wrapper.appendChild(ensureImageAnchor(node, group))
              }
            })
            if (!inserted) continue
            for (const block of run) {
              if (movedNodes.has(block)) continue
              if (block.parentNode) block.remove()
            }
          }
          continue
        }

        if (mediaItems.length < 2) {
          if (mediaItems.length === 1) {
            const firstBlock = run[0]
            const only = mediaItems[0]?.node
            if (firstBlock?.parentNode && only) {
              const isPlainImage = isPlainImageNode(only)
              const wrapper = document.createElement('div')
              wrapper.className = isPlainImage ? 'single-media inline-image-thumb' : 'single-media'
              firstBlock.parentNode.insertBefore(wrapper, firstBlock)
              wrapper.appendChild(isPlainImage ? ensureImageAnchor(only, 'inline-image') : only)

              if (firstBlock.tagName.toLowerCase() === 'p') firstBlock.remove()

              if (isPlainImage) continue

              const applyPortrait = (w: number, h: number) => {
                if (w > 0 && h > 0 && h > w) {
                  wrapper.classList.add('ar-11')
                  const tagName = (only as Element).tagName.toLowerCase()
                  if (tagName === 'img') {
                    const img = only as HTMLImageElement
                    img.style.width = '100%'
                    img.style.height = '100%'
                    img.style.objectFit = 'contain'
                  } else if (tagName === 'video') {
                    const vid = only as HTMLVideoElement
                    vid.style.width = '100%'
                    vid.style.height = '100%'
                    vid.style.objectFit = 'contain'
                  } else if (tagName === 'a') {
                    const img = (only as HTMLAnchorElement).querySelector('img') as HTMLImageElement | null
                    if (img) {
                      img.style.width = '100%'
                      img.style.height = '100%'
                      img.style.objectFit = 'contain'
                    }
                  }
                }
              }

              const tagName = (only as Element).tagName.toLowerCase()
              if (tagName === 'img') {
                const img = only as HTMLImageElement
                if (img.complete && img.naturalWidth && img.naturalHeight) applyPortrait(img.naturalWidth, img.naturalHeight)
                else img.addEventListener('load', () => applyPortrait(img.naturalWidth, img.naturalHeight), { once: true })
              } else if (tagName === 'a') {
                const img = (only as HTMLAnchorElement).querySelector('img') as HTMLImageElement | null
                if (img) {
                  if (img.complete && img.naturalWidth && img.naturalHeight) applyPortrait(img.naturalWidth, img.naturalHeight)
                  else img.addEventListener('load', () => applyPortrait(img.naturalWidth, img.naturalHeight), { once: true })
                }
              } else if (tagName === 'video') {
                const vid = only as HTMLVideoElement
                const runCheck = () => applyPortrait(vid.videoWidth, vid.videoHeight)
                if (vid.readyState >= 1 && vid.videoWidth && vid.videoHeight) runCheck()
                else vid.addEventListener('loadedmetadata', runCheck, { once: true })
              }
            }
          }
          continue
        }

        const grid = document.createElement('div');
        const count = mediaItems.length;
        const cols = count === 2 || count === 4 ? 2 : Math.min(3, count);
        grid.className = `image-grid cols-${cols}`;
        const group = `grid-${Math.random().toString(36).slice(2)}`;

        const firstBlock = run[0];
        if (firstBlock.parentNode) firstBlock.parentNode.insertBefore(grid, firstBlock);

        for (const { node } of mediaItems) {
           const item = document.createElement('div');
           item.className = 'image-grid-item';

           const tagName = (node as Element).tagName.toLowerCase();
           if (tagName === 'video') {
               const vid = node as HTMLVideoElement;
               vid.style.width = '100%';
               vid.style.height = '100%';
               vid.style.objectFit = 'cover';
               item.appendChild(vid);
            } else if ((node as Element).classList.contains('video-wrapper')) {
               const wrapper = node as HTMLElement;
               wrapper.style.width = '100%';
               wrapper.style.height = '100%';
               item.appendChild(wrapper);
            } else if (tagName === 'a') {
               const a = node as HTMLAnchorElement;
               a.setAttribute('data-fancybox', group);
               const img = a.querySelector('img');
               if (img && !a.getAttribute('href')) a.setAttribute('href', img.src);
               item.appendChild(a);
            } else if (tagName === 'img') {
               const img = node as HTMLImageElement;
               const a = document.createElement('a');
               a.setAttribute('href', img.src);
               a.setAttribute('data-fancybox', group);
               a.appendChild(img);
               item.appendChild(a);
            }
            grid.appendChild(item);
         }

         for (const block of run) {
            if (block.tagName.toLowerCase() === 'p' || isSingleMediaWrapper(block)) block.remove();
         }

         const updateGridUniformity = () => {
           const items = Array.from(grid.querySelectorAll('.image-grid-item')) as HTMLElement[];
           let landscapeCount = 0;
           let portraitCount = 0;
           let squareCount = 0;

           items.forEach(item => {
             const img = item.querySelector('img');
             const vid = item.querySelector('video');
             const wrapper = item.querySelector('.video-wrapper');

             if (wrapper) {
               landscapeCount++;
             } else if (img) {
               if (img.complete && img.naturalWidth) {
                 const r = img.naturalWidth / img.naturalHeight;
                 if (Math.abs(r - 1) < 0.15) squareCount++; // 宽松判定方形
                 else if (r > 1) landscapeCount++;
                 else portraitCount++;
               }
             } else if (vid) {
               if (vid.readyState >= 1) {
                 const r = vid.videoWidth / vid.videoHeight;
                 if (Math.abs(r - 1) < 0.15) squareCount++;
                 else if (r > 1) landscapeCount++;
                 else portraitCount++;
               }
             }
           });

           let targetClass = 'ar-11';
           const typesPresent = [landscapeCount > 0, portraitCount > 0, squareCount > 0].filter(Boolean).length;

           if (typesPresent > 1) {
             targetClass = 'ar-11'; // 混合类型强制方形，确保对齐
           } else if (landscapeCount > 0) {
             targetClass = 'ar-169';
           } else if (portraitCount > 0) {
             targetClass = 'ar-34';
           } else if (squareCount > 0) {
             targetClass = 'ar-11';
           } else {
             targetClass = 'ar-169'; // 默认
           }

           items.forEach(item => {
             item.classList.remove('ar-169', 'ar-34', 'ar-11');
             item.classList.add(targetClass);
           });
         };

         grid.querySelectorAll('img').forEach((imgEl) => {
           const img = imgEl as HTMLImageElement;
           if (img.complete) updateGridUniformity();
           else img.addEventListener('load', updateGridUniformity);
         });

         grid.querySelectorAll('video').forEach((vidEl) => {
           const vid = vidEl as HTMLVideoElement;
           if (vid.readyState >= 1) updateGridUniformity();
           else vid.addEventListener('loadedmetadata', updateGridUniformity);
         });

         grid.querySelectorAll('.video-wrapper').forEach(() => {
           updateGridUniformity();
         });

         // 初始执行一次
         updateGridUniformity();
     }
  }
};


export const applyImageLoadingPlaceholders = (root: HTMLElement | null) => {
  if (!root) return;
  const imgs = Array.from(root.querySelectorAll('img')) as HTMLImageElement[];
  imgs.forEach((img) => {
    if (img.closest('.github-card')) return;
    if (img.dataset.siteAttachmentKind) return;
    const container = (img.closest('.image-grid-item') || img.parentElement || root) as HTMLElement;
    const needPlaceholder = !img.complete || !(img.naturalWidth && img.naturalHeight);
    if (!needPlaceholder) return;
    const ph = document.createElement('div');
    ph.className = 'image-loading-placeholder';
    ph.textContent = '图片正在加载中，请稍后';
    const ref = img.closest('.image-grid-item') ? (img.closest('.image-grid-item') as HTMLElement).firstChild : img;
    container.insertBefore(ph, ref as Node);
    img.style.opacity = '0';
    const onLoad = () => {
      img.style.opacity = '1';
      ph.remove();
    };
    const onError = () => {
      ph.textContent = '图片加载失败';
      ph.classList.add('image-loading-error');
      img.style.display = 'none';
    };
    img.addEventListener('load', onLoad, { once: true });
    img.addEventListener('error', onError, { once: true });
  });
};
