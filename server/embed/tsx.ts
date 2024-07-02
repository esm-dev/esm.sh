/*! 🚀 esm.sh/tsx
 *
 * Add `<script src="https://esm.sh/tsx" type="module"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const modUrl = new URL(import.meta.url);
const kScript = "script";
const kImportmap = "importmap";
const loaders = ["jsx", "ts", "tsx", "babel"];
const importMapSupported = HTMLScriptElement.supports?.(kImportmap);
const imports: Record<string, string> = {};
const scopes: Record<string, typeof imports> = {};
const tsxScripts: { el: HTMLElement; loader: string; code: string }[] = [];

// lookup tsx scripts
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
      const code = el.textContent!.trim();
      if (code.length > 128 * 1024) {
        console.warn("[esm.sh/tsx] reach 128KB limit:", el);
      } else {
        if (loader === "babel") {
          loader = "jsx";
        }
        tsxScripts.push({ el, loader, code });
      }
    }
  }
});

// transform and insert tsx scripts
const importMap = stringify({ imports, scopes });
tsxScripts.forEach(async ({ el, loader, code }, idx) => {
  const filename = "source." + loader;
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      // @ts-expect-error `$TARGET` is injected by esbuild
      new TextEncoder().encode(loader + code + importMap + $TARGET + "false"),
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
    const { origin } = modUrl;
    const res = await fetch(origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const res = await fetch(origin + "/transform", {
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
