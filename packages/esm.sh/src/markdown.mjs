import init, { parse, ParseFlags } from "../vendor/markdown-wasm@1.2.0.mjs";

export default {
  get ready() {
    return this._waiting || (this._waiting = init());
  },
  parse,
  ParseFlags,
  /**
   * @param {Response} res
   * @param {object|undefined} options
   * @returns {Promise<Response>}
   */
  async transform(res, options) {
    await this.ready;
    const headers = new Headers(res.headers);
    const outdatedHeaders = [
      "content-encoding",
      "content-length",
      "etag",
    ];
    for (const key of outdatedHeaders) {
      headers.delete(key);
    }
    headers.set("content-type", "text/html;charset=UTF-8");
    return new Response(
      parse(new Uint8Array(await res.arrayBuffer()), {
        ...options,
        bytes: true,
      }),
      {
        status: res.status,
        headers,
      },
    );
  },
};
