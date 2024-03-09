/*! 🚀 esm.sh/run
 *
 * Add `<script src="https://esm.sh/run" type="module"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const modUrl = new URL(import.meta.url);
const kScript = "script";
const kImportmap = "importmap";
const loaders = ["js", "jsx", "ts", "tsx", "babel"];
const importMapSupported = HTMLScriptElement.supports?.(kImportmap);
const imports: Record<string, string> = {};
const scopes: Record<string, typeof imports> = {};
const runScripts: { el: HTMLElement; loader: string; source: string }[] = [];

// lookup run scripts
d.querySelectorAll(kScript).forEach((el) => {
  let loader: string | null = null;
  if (el.type === kImportmap) {
    const v = JSON.parse(el.textContent!);
    for (const k in v.imports) {
      if (!importMapSupported || k === "@jsxImportSource") {
        imports[k] = v.imports[k];
      }
    }
    if (!importMapSupported) {
      Object.assign(scopes, v.scopes);
    }
  } else if (el.type.startsWith("text/")) {
    loader = el.type.slice(5);
    if (loaders.includes(loader)) {
      const source = el.textContent!.trim();
      if (source.length > 64 * 1024) {
        console.warn("ignore", el, "reach 64KB limit.");
      } else {
        runScripts.push({ el, loader, source });
      }
    }
  }
});

// transform and insert run scripts
const importMap = stringify({ imports, scopes });
runScripts.forEach(async ({ el, loader, source }, idx) => {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(loader + source + importMap),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = "esm.sh/run:" + idx;
  const hashCacheKey = jsCacheKey + ".hash";
  let js = l.getItem(jsCacheKey);
  if (js && l.getItem(hashCacheKey) !== hash) {
    js = null;
  }
  if (!js) {
    const { origin } = modUrl;
    const res = await fetch(origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const res = await fetch(origin + "/transform", {
        method: "POST",
        body: stringify({ loader, source, importMap, hash }),
      });
      const { code, error } = await res.json();
      if (error) {
        throw new Error(error.message);
      }
      js = code;
    }
    l.setItem(jsCacheKey, js!);
    l.setItem(hashCacheKey, hash);
  }
  const script = d.createElement(kScript);
  script.type = "module";
  script.textContent = js!;
  el.replaceWith(script);
});
