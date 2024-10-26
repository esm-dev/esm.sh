import { assertEquals, assertStringIncludes } from "jsr:@std/assert";

Deno.test("transform api", async () => {
  const options = {
    lang: "jsx",
    code: `
      import { renderToString } from "preact-render-to-string";
      export default () => renderToString(<h1>Hello world!</h1>);
    `,
    target: "es2022",
    importMap: {
      imports: {
        "@jsxRuntime": "https://preact@10.13.2",
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
  assertStringIncludes(transformOut.code, `"https://preact@10.13.2/jsx-runtime"`);
  assertStringIncludes(transformOut.code, `"https://esm.sh/preact-render-to-string6.0.2"`);
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

Deno.test("transform api(remote module, react)", async () => {
  const im = "y" + btoaUrl("/esm-run-demo/react/");
  const res1 = await fetch(`http://localhost:8080/https://ije.github.io/esm-run-demo/react/main.tsx?im=${im}`);
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  assertEquals(res1.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const js1 = await res1.text();
  assertStringIncludes(js1, 'from"https://esm.sh/react-dom@18.3.1/client";');
  assertStringIncludes(js1, 'from"https://esm.sh/react@18.3.1/jsx-runtime";');
  assertStringIncludes(js1, '("h1",{children:"Hello World!"})');
});

Deno.test("transform api(remote module, preact)", async () => {
  const im = "y" + btoaUrl("/esm-run-demo/preact/");
  const res1 = await fetch(`http://localhost:8080/https://ije.github.io/esm-run-demo/preact/main.tsx?im=${im}`);
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  assertEquals(res1.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const js1 = await res1.text();
  assertStringIncludes(js1, 'from"preact";');
  assertStringIncludes(js1, 'from"https://esm.sh/preact@10.24.1/jsx-runtime";');
  assertStringIncludes(js1, '("h1",{children:"Hello World!"})');
});

Deno.test("transform api(remote module, vue)", async () => {
  const im = "y" + btoaUrl("/esm-run-demo/vue/");
  const res1 = await fetch(`http://localhost:8080/https://ije.github.io/esm-run-demo/vue/main.ts?im=${im}`);
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  assertEquals(res1.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const js1 = await res1.text();
  assertStringIncludes(js1, 'from"vue";');
  assertStringIncludes(js1, "h1[data-v-");
});

Deno.test("transform api(remote module, svelte)", async () => {
  const im = "y" + btoaUrl("/esm-run-demo/svelte/");
  const res1 = await fetch(`http://localhost:8080/https://ije.github.io/esm-run-demo/svelte/main.ts?im=${im}`);
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  assertEquals(res1.headers.get("cache-control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const js1 = await res1.text();
  assertStringIncludes(js1, 'from"https://esm.sh/svelte@4.2.19/internal";');
  assertStringIncludes(js1, "color:#ff4000");
});

Deno.test("transform api(remote module, import maps is not supported)", async () => {
  const im = "N" + btoaUrl("/esm-run-demo/preact/");
  const res1 = await fetch(`http://localhost:8080/https://ije.github.io/esm-run-demo/preact/main.tsx?im=${im}`);
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "application/javascript; charset=utf-8");
  assertEquals(res1.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const js1 = await res1.text();
  assertStringIncludes(js1, 'from"https://esm.sh/preact@10.24.1";');
  assertStringIncludes(js1, 'from"https://esm.sh/preact@10.24.1/jsx-runtime";');
});

Deno.test("transform api(uno.css)", async () => {
  const res1 = await fetch(
    "http://localhost:8080/uno.css?p="
      + btoaUrl("https://ije.github.io/esm-run-demo/unocss/")
      + "&c="
      + btoaUrl("./uno.css"),
  );
  assertEquals(res1.status, 200);
  assertEquals(res1.headers.get("Content-Type"), "text/css; charset=utf-8");
  assertEquals(res1.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
  assertStringIncludes(res1.headers.get("Vary")!, "User-Agent");
  const css1 = await res1.text();
  assertStringIncludes(css1, "time,mark,audio,video{"); // eric-meyer reset css
  assertStringIncludes(css1, ".btn{");
  assertStringIncludes(css1, ".btn:hover{");
  assertStringIncludes(css1, "background-color:rgb(59 130 246");
  assertStringIncludes(css1, "@keyframes spin");
  assertStringIncludes(css1, ".btn:hover{animation:spin 1s ease infinite}");
  assertStringIncludes(css1, "@font-face{");
  assertStringIncludes(css1, "https://fonts.gstatic.com/s/inter/");
  assertStringIncludes(css1, "font-family:Inter,ui-sans-serif,");
  assertStringIncludes(css1, '.i-carbon-logo-github{--un-icon:url("data:image/svg+xml;utf8,');
  assertStringIncludes(css1, ".all\\:transition-40 *{");
});

function btoaUrl(url: string): string {
  return btoa(url).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

async function computeHash(input: string): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}
