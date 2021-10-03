/* deno mod bundle
 * entry: deno.land/std/node/stream/web.ts
 * version: 0.109.0
 *
 *   $ git clone https://github.com/denoland/deno_std
 *   $ cd deno_std/node
 *   $ esbuild stream/web.ts --target=esnext --format=esm --bundle --outfile=deno_std_node_stream_web.js
 */

// stream/web.ts
var {
  ReadableStream,
  ReadableStreamDefaultReader,
  ReadableStreamDefaultController,
  WritableStream,
  WritableStreamDefaultWriter,
  TransformStream
} = globalThis;
export {
  ReadableStream,
  ReadableStreamDefaultController,
  ReadableStreamDefaultReader,
  TransformStream,
  WritableStream,
  WritableStreamDefaultWriter
};
