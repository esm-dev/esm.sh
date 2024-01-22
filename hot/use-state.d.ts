declare global {
  interface HotAPI {
    state(
      init: Record<string, unknown> | Promise<Record<string, unknown>>,
    ): void;
  }
}

export {};
