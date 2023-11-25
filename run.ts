/*! esm.sh run
 *
 * Add `<script type="module" src="https://esm.sh/run" defer></script>` to your HTML to run jsx/tsx in browser without build.
 *
 */

const d = document;
const l = localStorage;
const KB = 1024;
const kRun = "esm.sh/run";
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
let imImports: Record<string, string> = {};

// lookup run scripts
d.querySelectorAll(kScript).forEach((el) => {
  let loader: string | null = null;
  if (el.type === kImportmap) {
    const v = JSON.parse(el.innerHTML).imports;
    for (const k in v) {
      imImports[k] = v[k];
    }
    if (HTMLScriptElement.supports?.(kImportmap)) {
      imImports = { [kJsxImportSource]: imImports[kJsxImportSource] };
    }
  } else {
    loader = loaders[el.type];
    if (loader) {
      const code = el.innerHTML;
      if (code.length > 100 * KB) {
        throw new Error(kRun + " " + code.length + " bytes exceeded limit.");
      }
      runScripts.push({ loader, code });
    }
  }
});

// transform and insert scripts
runScripts.forEach(async (input, idx) => {
  const { origin } = new URL(import.meta.url);
  const stringify = JSON.stringify;
  const imports = stringify(imImports);
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(
        input.loader + input.code + imports,
      ),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = kRun + "/" + idx;
  const hashCacheKey = jsCacheKey + "/hash";
  let js = l.getItem(jsCacheKey);
  if (js && l.getItem(hashCacheKey) !== hash) {
    js = null;
  }
  if (!js) {
    const res = await fetch(origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const { code, error } = await fetch(origin + "/transform", {
        method: "POST",
        body: stringify({ ...input, imports, hash }),
      }).then((res) => res.json());
      if (error) {
        throw new Error(kRun + " " + error.message);
      }
      js = code;
    }
    l.setItem(jsCacheKey, js!);
    l.setItem(hashCacheKey, hash);
  }
  const script = d.createElement(kScript);
  script.type = "module";
  script.innerHTML = js!;
  d.body.appendChild(script);
});
