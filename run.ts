/*! esm.sh run
 *
 * Add `<script type="module" src="https://esm.sh/run" defer></script>` to your HTML to run jsx/tsx in browser without build.
 *
 */

/// <reference lib="dom" />

const d = document;
const l = localStorage;
const kImportmap = "importmap";
const kJsxImportSource = "@jsxImportSource";
const kScript = "script";
const loaders: Record<string, string> = {
  "text/jsx": "jsx",
  "text/babel": "tsx",
  "text/tsx": "tsx",
  "text/ts": "ts",
};

const runScripts: { loader: string; code: string }[] = [];
let imports: Record<string, string> = {};

// lookup run scripts
d.querySelectorAll(kScript).forEach((el) => {
  let loader: string | null = null;
  if (el.type === kImportmap) {
    const v = JSON.parse(el.innerHTML).imports;
    for (const k in v) {
      imports[k] = v[k];
    }
    if (HTMLScriptElement.supports?.(kImportmap)) {
      imports = { [kJsxImportSource]: imports[kJsxImportSource] };
    }
  } else {
    loader = loaders[el.type];
  }
  if (loader) {
    runScripts.push({ loader, code: el.innerHTML });
  }
});

// transform and insert scripts
runScripts.forEach(async (input, idx) => {
  const murl = new URL(import.meta.url);
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(
        input.loader + input.code +
          (imports
            ? Object.keys(imports).sort().map((k) => k + imports![k]).join("")
            : ""),
      ),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = "esm.sh/run/" + idx;
  const hashCacheKey = jsCacheKey + "/hash";
  let js = l.getItem(jsCacheKey);
  if (js && l.getItem(hashCacheKey) !== hash) {
    js = null;
  }
  if (!js) {
    const res = await fetch(murl.origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const { transform } = await import(`./build`);
      const ret = await transform({ ...input, imports, hash });
      js = ret.code;
    }
    l.setItem(jsCacheKey, js!);
    l.setItem(hashCacheKey, hash);
  }
  const script = d.createElement(kScript);
  script.type = "module";
  script.innerHTML = js!;
  d.body.appendChild(script);
});
