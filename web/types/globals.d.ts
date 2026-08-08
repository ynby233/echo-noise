declare global {
  interface Window {
    Fancybox?: {
      bind: (selector: string, options?: Record<string, unknown>) => void;
      destroy: () => void;
    };
  }
}

export {};
