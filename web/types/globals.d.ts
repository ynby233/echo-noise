declare global {
  interface Window {
    Fancybox?: {
      bind: (selector: string, options?: Record<string, unknown>) => void;
      destroy: () => void;
    };
    Waline?: {
      init: (options: Record<string, unknown>) => unknown;
    };
  }
}

export {};