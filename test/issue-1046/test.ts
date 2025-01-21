import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("issue #1046", async () => {
  {
    const res = await fetch("http://localhost:8080/@statistikzh/leu@0.13.1/leu-dropdown.js", {
      headers: { "user-agent": "i'm a browser" },
    });
    assertEquals(res.status, 200);
    assertStringIncludes(await res.text(), `export * from "/@statistikzh/leu@0.13.1/es2022/leu-dropdown.mjs`);
  }
  {
    const res = await fetch("http://localhost:8080/@statistikzh/leu@0.13.1/leu-dropdown", { headers: { "user-agent": "i'm a browser" } });
    assertEquals(res.status, 200);
    assertStringIncludes(await res.text(), `export * from "/@statistikzh/leu@0.13.1/es2022/leu-dropdown.mjs`);
  }
});
