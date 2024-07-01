import { assertStringIncludes } from "jsr:@std/assert";

Deno.test("bundle deps", async () => {
  const code = await fetch("http://localhost:8080/buffer@6.0.3/es2022/buffer.bundle.mjs").then((res) => res.text());
  assertStringIncludes(
    code,
    `"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"`,
  );
});

Deno.test("no bundle", async () => {
  const code = await fetch("http://localhost:8080/@pyscript/core@0.3.4/es2022/dist/py-terminal-XWbSa71s.nobundle.js")
    .then((res) => res.text());
  assertStringIncludes(code, "./core.nobundle.js");
  assertStringIncludes(code, "./error-96hMSEw8.nobundle.js");
});
