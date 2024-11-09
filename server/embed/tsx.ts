/*! 🚀 esm.sh/tsx
 *
 * Add `<script type="module" src="https://esm.sh/tsx"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const loaders = new Set(["jsx", "ts", "tsx", "babel"]);
const target = "$TARGET"; // `$TARGET` is injected at build time
const isLocalhost = false;

function tsx() {
  let tsxScripts: { el: HTMLElement; lang: string; code: string }[] = [];
  let importMap: Record<string, object> = {};
  let tsx: Promise<{ transform: (filename: string, code: string, options: Record<string, unknown>) => { code: string } }>;

  // lookup import map and tsx scripts
  d.querySelectorAll("script").forEach((el) => {
    const { type, textContent: code } = el;
    if (!code?.trim()) return;
    if (type === "importmap") {
      const v = JSON.parse(code!);
      if (v) {
        v.$support = HTMLScriptElement.supports("importmap");
        importMap = v;
      }
    } else if (type.startsWith("text/")) {
      const lang = type.slice(5);
      if (loaders.has(lang)) {
        if (code.length > 128 * 1024) {
          throw new Error("[esm.sh/tsx] reach 128KB limit");
        }
        tsxScripts.push({ el, lang: lang === "babel" ? "tsx" : lang, code });
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
    const script = d.createElement("script");
    let js: string | null | undefined;
    try {
      js = l.getItem(jsCacheKey);
      if (js && l.getItem(hashCacheKey) !== hash) {
        js = null;
      }
    } catch {
      // localStorage is disallowed
    }
    if (!js) {
      if (isLocalhost) {
        const { transform } = await (tsx ?? (tsx = initTsx()));
        const ret = transform("script-" + idx + "." + lang, code, { target, importMap, minify: true, sourceMap: "inline" });
        js = ret.code;
      } else {
        const res = await fetch(urlFromCurrentModule(`/+${hash}.mjs`));
        if (res.ok) {
          js = await res.text();
        } else {
          const res = await fetch(urlFromCurrentModule("/transform"), {
            method: "POST",
            body: stringify({ lang, code, target, importMap, minify: true }),
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
    script.type = "module";
    script.textContent = js!;
    el.replaceWith(script);
  });
}

async function initTsx() {
  const pkg = "/@esm.sh/tsx@1.0.2";
  const [m, w] = await Promise.all([
    import(pkg + "/$TARGET/@esm.sh/tsx.mjs"),
    fetch(urlFromCurrentModule(pkg + "/pkg/tsx_bg.wasm")),
  ]);
  await m.default(w);
  return m;
}

function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
}

tsx();
