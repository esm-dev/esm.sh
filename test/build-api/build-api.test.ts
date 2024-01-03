import {
  assertEquals,
  assertStringIncludes,
} from "https://deno.land/std@0.180.0/testing/asserts.ts";

import { build, esm } from "http://localhost:8080/build";

Deno.test("build api", async (t) => {
  let url = "";
  let bundleUrl = "";
  await t.step("build", async () => {
    const ret = await fetch("http://localhost:8080/build", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        code: `/* @jsx h */
          import { h } from "npm:preact@10.13.2";
          import { renderToString } from "npm:preact-render-to-string@6.0.2";
          export default () => renderToString(<h1>Hello world!</h1>);
        `,
        loader: "jsx",
      }),
    }).then((r) => r.json());
    if (ret.error) {
      throw new Error(`<${ret.error.status}> ${ret.error.message}`);
    }
    url = ret.url;
    bundleUrl = ret.bundleUrl;
    assertStringIncludes(url, "/~");
    assertStringIncludes(bundleUrl, "?bundle");
  });

  await t.step("import published module", async () => {
    const { default: render1 } = await import(url);
    const { default: render2 } = await import(bundleUrl);
    assertEquals(render1(), "<h1>Hello world!</h1>");
    assertEquals(render2(), "<h1>Hello world!</h1>");
  });
});

Deno.test("build api (with options)", async () => {
  const ret = await fetch("http://localhost:8080/build", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      dependencies: {
        preact: "^10.13.2",
      },
      code: `
        export { h } from "preact";
      `,
      types: `
        export { h } from "preact"
      `,
    }),
  }).then((r) => r.json());
  if (ret.error) {
    throw new Error(`<${ret.error.status}> ${ret.error.message}`);
  }

  const mod = await import(ret.url);
  mod.h("h1", null, "Hello world!");
  assertEquals(typeof mod.h, "function");
});

Deno.test("build api (transformOnly)", async () => {
  const ret = await fetch("http://localhost:8080/transform", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      code: `
        const n:number = 42;
      `,
    }),
  }).then((r) => r.json());
  if (ret.error) {
    throw new Error(`<${ret.error.status}> ${ret.error.message}`);
  }
  assertEquals(ret.code, "var n=42;\n");
});

Deno.test("build api (transform with hash)", async () => {
  const options = {
    loader: "ts",
    code: `
      const n:number = 42;
    `,
    importMap: `{"imports":{}}`,
    hash: "",
  };
  options.hash = await computeHash(
    options.loader + options.code + options.importMap,
  );
  const ret = await fetch("http://localhost:8080/transform", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options),
  }).then((r) => r.json());
  if (ret.error) {
    throw new Error(`<${ret.error.status}> ${ret.error.message}`);
  }
  assertEquals(ret.code, "var n=42;\n");

  const res = await fetch(`http://localhost:8080/+${options.hash}.mjs`);
  assertEquals(res.status, 200);
  assertEquals(await res.text(), "var n=42;\n");
});

Deno.test("build api (use sdk)", async (t) => {
  await t.step("use `build` function", async () => {
    const ret = await build(`export default "Hello world!";`);
    const { default: message } = await import(ret.url);
    assertEquals(message, "Hello world!");
  });

  await t.step("use `esm` tag function", async () => {
    const message = "Hello world!";
    const mod = await esm<{ default: string }>`export default ${
      JSON.stringify(message)
    };`;
    assertEquals(mod.default, message);
  });
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
