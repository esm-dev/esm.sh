import { assertEquals } from "https://deno.land/std@0.220.0/assert/mod.ts";

import { createSSRApp, h } from "http://localhost:8080/vue@3.2.47";
import { renderToString, renderToWebStream } from "http://localhost:8080/vue@3.2.47/server-renderer";

const app = createSSRApp({
  data: () => ({ msg: "The Progressive JavaScript Framework" }),
  render() {
    return h("div", this.msg);
  },
});

Deno.test("Vue (3.2)", async () => {
  assertEquals(await renderToString(app), "<div>The Progressive JavaScript Framework</div>");
  assertEquals(await new Response(renderToWebStream(app)).text(), "<div>The Progressive JavaScript Framework</div>");
});
