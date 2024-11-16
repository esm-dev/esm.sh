import { assertEquals } from "jsr:@std/assert";

Deno.test("Vue SSR", async () => {
  const { createSSRApp, h } = await import("http://localhost:8080/vue@3.2.47");
  const { renderToString, renderToWebStream } = await import("http://localhost:8080/vue@3.2.47/server-renderer");

  const app = createSSRApp({
    data: () => ({ msg: "The Progressive JavaScript Framework" }),
    render() {
      return h("div", this.msg);
    },
  });
  assertEquals(await renderToString(app), "<div>The Progressive JavaScript Framework</div>");
  assertEquals(await new Response(renderToWebStream(app)).text(), "<div>The Progressive JavaScript Framework</div>");
});
