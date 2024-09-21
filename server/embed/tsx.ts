/*! 🚀 esm.sh/tsx
 *
 * Add `<script type="module" src="https://esm.sh/tsx"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const loaders = new Set(["jsx", "ts", "tsx", "babel"]);
const isLocalhost = ["localhost", "127.0.0.1"].includes(location.hostname);
const target: string = "$TARGET"; // `$TARGET` is injected at build time

let tsxScripts: { el: HTMLElement; lang: string; code: string }[] = [];
let importMap: Record<string, any> = {};
let esmCompiler: Promise<any> | null = null;

// lookup import map and tsx scripts
d.querySelectorAll("script").forEach((el) => {
  const { type, textContent } = el;
  if (type === "importmap") {
    const v = JSON.parse(textContent!);
    if (v && v.imports) {
      importMap.imports = v.imports;
    }
  } else if (type.startsWith("text/")) {
    let lang = type.slice(5);
    if (loaders.has(lang)) {
      const code = textContent!.trim();
      if (code.length > 128 * 1024) {
        console.warn("[esm.sh/tsx] reach 128KB limit:", el);
      } else {
        if (lang === "babel") {
          lang = "tsx";
        }
        tsxScripts.push({ el, lang, code });
      }
    }
  }
});

// transform and insert tsx scripts
tsxScripts.forEach(async ({ el, lang, code }, idx) => {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(lang + code + target + stringify(importMap) + "true"),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = "esm.sh/tsx." + idx;
  const hashCacheKey = jsCacheKey + ".hash";
  let js: string | null;
  try {
    js = l.getItem(jsCacheKey);
    if (js && l.getItem(hashCacheKey) !== hash) {
      js = null;
    }
  } catch {
    // localStorage is disallowed
    js = null;
  }
  if (!js) {
    if (isLocalhost) {
      const { transform } = await (esmCompiler ?? (esmCompiler = loadEsmCompiler()));
      const ret = transform("source." + lang, code, { importMap, target, minify: true, sourceMap: "inline" });
      js = ret.code;
    } else {
      const res = await fetch(urlFromCurrentModule(`/+${hash}.mjs`));
      if (res.ok) {
        js = await res.text();
      } else {
        const res = await fetch(urlFromCurrentModule("/transform"), {
          method: "POST",
          body: stringify({ lang, code, importMap, target, minify: true }),
        });
        const ret: any = await res.json();
        if (ret.error) {
          throw new Error(ret.error.message);
        }
        js = ret.code;
      }
    }
    try {
      l.setItem(jsCacheKey, js!);
      l.setItem(hashCacheKey, hash);
    } catch {
      // localStorage is disallowed
    }
  }
  const script = d.createElement("script");
  script.type = "module";
  script.textContent = js!;
  el.replaceWith(script);
});

async function loadEsmCompiler() {
  const pkg = "/esm-compiler@0.9.1";
  const m = await import(pkg + "/$TARGET/esm-compiler.mjs");
  await m.default(urlFromCurrentModule(pkg + "/pkg/esm_compiler_bg.wasm"));
  return m;
}

function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
}
