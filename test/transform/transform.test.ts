import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("transform API", async () => {
  const options = {
    lang: "jsx",
    code: `
      import { renderToString } from "preact-render-to-string";
      export default () => renderToString(<h1>esm.sh</h1>);
    `,
    target: "es2022",
    importMap: {
      imports: {
        "preact/jsx-runtime": "https://esm.sh/preact@10.13.2/jsx-runtime",
        "preact-render-to-string": "https://esm.sh/preact-render-to-string6.0.2",
      },
    },
    sourceMap: "external",
    minify: true,
  };
  const hash = await computeHash(
    options.lang + options.code + options.target + JSON.stringify(options.importMap) + options.sourceMap + options.minify,
  );
  const res1 = await fetch("http://localhost:8080/transform", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options),
  });
  assertEquals(res1.status, 200);
  const transformOut = await res1.json();
  assertStringIncludes(transformOut.code, `"preact/jsx-runtime"`);
  assertStringIncludes(transformOut.code, `("h1"`);
  assertStringIncludes(transformOut.code, `//# sourceMappingURL=+${hash}.mjs.map`);
  assertStringIncludes(transformOut.map, `"mappings":`);

  const res2 = await fetch(`http://localhost:8080/+${hash}.mjs`);
  assertEquals(res2.status, 200);
  assertEquals(res2.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  const js = await res2.text();
  assertEquals(js, transformOut.code);

  const res3 = await fetch(`http://localhost:8080/+${hash}.mjs.map`);
  assertEquals(res3.status, 200);
  assertEquals(res3.headers.get("Content-Type"), "application/json; charset=utf-8");
  const map = await res3.text();
  assertEquals(map, transformOut.map);
});

async function computeHash(input: string): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}
