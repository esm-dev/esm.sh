import init, { HTMLRewriter } from "../vendor/html-rewriter-wasm@0.4.1.mjs";

// HTMLRewriter compatibility class
// https://developers.cloudflare.com/workers/runtime-apis/html-rewriter
globalThis.HTMLRewriter = class {
  static waiting = init();
  constructor() {
    const w = this.w = new HTMLRewriter((chunk) => {
      this.c.enqueue(chunk);
    });
    this.t = new TransformStream({
      start: (controller) => {
        this.c = controller;
      },
      transform: async (chunk) => {
        try {
          await w.write(await chunk);
        } finally {
        }
      },
      flush: async () => {
        try {
          await w.end();
        } finally {
          w.free();
        }
      },
    });
  }
  on(selector, handlers) {
    this.w.on(selector, handlers);
    return this;
  }
  onDocument(handlers) {
    this.w.onDocument(handlers);
    return this;
  }
  write(chunk) {
    return this.w.write(chunk);
  }
  end() {
    return this.w.end();
  }
  free() {
    this.w.free();
  }
  /**
   * @param {Response} response
   * @returns {Response}
   */
  transform(response) {
    const headers = new Headers(response.headers);
    const outdatedHeaders = [
      "content-encoding",
      "content-length",
      "last-modified",
      "etag",
    ];
    for (const key of outdatedHeaders) {
      headers.delete(key);
    }
    return new Response(response.body?.pipeThrough(this.t), {
      status: response.status,
      statusText: response.statusText,
      headers,
    });
  }
};
