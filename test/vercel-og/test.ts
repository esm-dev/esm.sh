import { assert, assertEquals } from "jsr:@std/assert";
import { ImageResponse } from "esm.sh/@vercel/og@0.6.2?conditions=edge";

Deno.test("@vercel/og renders a PNG with WASM modules", async () => {
  const response = new ImageResponse(
    { type: "div", key: null, props: { children: "Hello" } },
    { width: 100, height: 100 },
  );
  assertEquals(response.status, 200);
  assertEquals(response.headers.get("content-type"), "image/png");
  const png = new Uint8Array(await response.arrayBuffer());
  assert(png.length > 24);
  assertEquals(
    png.slice(0, 8),
    new Uint8Array([137, 80, 78, 71, 13, 10, 26, 10]),
  );
  const view = new DataView(png.buffer);
  assertEquals(view.getUint32(16), 100);
  assertEquals(view.getUint32(20), 100);
});
