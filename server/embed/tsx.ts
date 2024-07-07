/*! 🚀 esm.sh/tsx
 *
 * Add `<script src="https://esm.sh/tsx" type="module"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const loaders = new Set(["jsx", "ts", "tsx", "babel"]);
const kScript = "script";

let tsxScripts: { el: HTMLElement; loader: string; code: string }[] = [];
let importMap: Record<string, any> = {};

// lookup import map and tsx scripts
d.querySelectorAll(kScript).forEach((el) => {
  const { type, textContent } = el;
  if (type === "importmap") {
    const v = JSON.parse(textContent!);
    if (v && typeof v === "object") {
      Object.assign(importMap, v);
    }
  } else if (type.startsWith("text/")) {
    let loader = type.slice(5);
    if (loaders.has(loader)) {
      const code = textContent!.trim();
      if (code.length > 128 * 1024) {
        console.warn("[esm.sh/tsx] reach 128KB limit:", el);
      } else {
        if (loader === "babel") {
          loader = "tsx";
        }
        tsxScripts.push({ el, loader, code });
      }
    }
  }
});

// transform and insert tsx scripts
tsxScripts.forEach(async ({ el, loader, code }, idx) => {
  const filename = "source." + loader;
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      // @ts-expect-error `$TARGET` is injected by esbuild
      new TextEncoder().encode(loader + code + stringify(importMap) + $TARGET + "false"),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = "esm.sh/tsx:" + idx;
  const hashCacheKey = jsCacheKey + ".hash";
  let js = l.getItem(jsCacheKey);
  if (js && l.getItem(hashCacheKey) !== hash) {
    js = null;
  }
  if (!js) {
    const res = await fetch(urlFromCurrentModule(`/+${hash}.mjs`));
    if (res.ok) {
      js = await res.text();
    } else {
      const res = await fetch(urlFromCurrentModule("/transform"), {
        method: "POST",
        // @ts-expect-error `$TARGET` is injected by esbuild
        body: stringify({ filename, code, importMap, target: $TARGET }),
      });
      const ret: any = await res.json();
      if (ret.error) {
        throw new Error(ret.error.message);
      }
      js = ret.code;
    }
    l.setItem(jsCacheKey, js!);
    l.setItem(hashCacheKey, hash);
  }
  const script = d.createElement(kScript);
  script.type = "module";
  script.textContent = js!;
  el.replaceWith(script);
});

function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
}
