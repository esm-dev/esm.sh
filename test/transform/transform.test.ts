import { assertEquals, assertStringIncludes } from "jsr:@std/assert";
import { contentType } from "jsr:@std/media-types";
import { join } from "jsr:@std/path";

Deno.test("transform", async (t) => {
  await t.step("transform api", async () => {
    const options = {
      lang: "jsx",
      code: `
        import { renderToString } from "preact-render-to-string";
        export default () => renderToString(<h1>esm.sh</h1>);
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

  const modUrl = new URL(import.meta.url);
  const demoRootDir = join(modUrl.pathname, "../../../cli/cmd/demo");
  const ac = new AbortController();

  Deno.serve({
    port: 8083,
    signal: ac.signal,
  }, async req => {
    let { pathname } = new URL(req.url);
    if (pathname.endsWith("/")) {
      pathname += "index.html";
    }
    try {
      const file = join(demoRootDir, pathname);
      const f = await Deno.open(file);
      return new Response(f.readable, {
        headers: {
          "Content-Type": contentType(pathname) ?? "application/octet-stream",
          "User-Agent": "es/2022",
        },
      });
    } catch (e) {
      if (e instanceof Deno.errors.NotFound) {
        return new Response("Not Found", { status: 404 });
      }
      return new Response("Internal Server Error", { status: 500 });
    }
  });

  await new Promise((resolve) => setTimeout(resolve, 100));

  await t.step("transform api(remote module, react)", async () => {
    const im = btoaUrl("/react/");
    const res = await fetch(`http://localhost:8080/http://localhost:8083/react/app/main.tsx?im=${im}`);
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    const js = await res.text();
    assertStringIncludes(js, 'from"https://esm.sh/react-dom@18.3.1/client";');
    assertStringIncludes(js, 'from"https://esm.sh/react@18.3.1/jsx-runtime";');
    assertStringIncludes(js, '("h1",{style:{color:"#61DAFB"},children:"esm.sh"})');
  });

  await t.step("transform api(remote module, preact)", async () => {
    const im = btoaUrl("/preact/");
    const res = await fetch(`http://localhost:8080/http://localhost:8083/preact/app/main.tsx?im=${im}`);
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    const js = await res.text();
    assertStringIncludes(js, 'from"https://esm.sh/preact@10.24.1";');
    assertStringIncludes(js, 'from"https://esm.sh/preact@10.24.1/jsx-runtime";');
    assertStringIncludes(js, '("h1",{style:{color:"#673AB8"},children:"esm.sh"})');
  });

  await t.step("transform api(remote module, vue)", async () => {
    const im = btoaUrl("/vue/");
    const res = await fetch(`http://localhost:8080/http://localhost:8083/vue/app/main.ts?im=${im}`);
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    const js = await res.text();
    assertStringIncludes(js, 'from"https://esm.sh/vue@3.5.8";');
    assertStringIncludes(js, "h1[data-v-");
    assertStringIncludes(js, "color: #42b883;");
    assertStringIncludes(js, 'globalThis.document.head.insertAdjacentHTML("beforeend",`<style>*{margin:0;padding:0;box-sizing:border-box}');
    assertStringIncludes(js, ">esm.sh</h1>");
  });

  await t.step("transform api(remote module, svelte)", async () => {
    const im = btoaUrl("/svelte/");
    const res = await fetch(`http://localhost:8080/http://localhost:8083/svelte/app/main.ts?im=${im}`);
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "application/javascript; charset=utf-8");
    assertEquals(res.headers.get("cache-control"), "public, max-age=31536000, immutable");
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    const js = await res.text();
    assertStringIncludes(js, 'from"https://esm.sh/svelte@5.1.12/internal/client?no-bundle";');
  });

  await t.step("transform api(uno.css)", async () => {
    const res = await fetch(
      "http://localhost:8080/uno.css?ctx="
        + btoaUrl("http://localhost:8083/with-unocss/vue/"),
    );
    assertEquals(res.status, 200);
    assertEquals(res.headers.get("Content-Type"), "text/css; charset=utf-8");
    assertEquals(res.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
    assertStringIncludes(res.headers.get("Vary")!, "User-Agent");
    const css = await res.text();
    assertStringIncludes(css, "time,mark,audio,video{"); // eric-meyer reset css
    assertStringIncludes(css, ".center-box{");
    assertStringIncludes(css, ".logo{");
    assertStringIncludes(css, ".logo:hover{");
    assertStringIncludes(css, "@font-face{");
    assertStringIncludes(css, "https://fonts.gstatic.com/s/inter/");
    assertStringIncludes(css, ".font-sans{font-family:Inter,");
    assertStringIncludes(css, '.i-tabler-brand-github{--un-icon:url("data:image/svg+xml;utf8,');
    assertStringIncludes(css, ".text-primary{--un-text-opacity:1;color:rgb(66 184 131 / var(--un-text-opacity))}");
    assertStringIncludes(css, ".text-gray-400{--un-text-opacity:1;color:rgb(156 163 175 / var(--un-text-opacity))}");
    assertStringIncludes(css, ".fw400{font-weight:400}.fw500{font-weight:500}.fw600{font-weight:600}");
    assertStringIncludes(css, ".all\\:transition-300 *{");
  });

  ac.abort();
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
