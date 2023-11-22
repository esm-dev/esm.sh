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
  "text/babel": "jsx",
  "text/tsx": "tsx",
  "text/ts": "ts",
};

const runScripts: { loader: string; code: string }[] = [];
let imports: Record<string, string> | undefined = undefined;

d.querySelectorAll(kScript).forEach((el) => {
  let loader: string | null = null;
  if (el.type === kImportmap) {
    imports = JSON.parse(el.innerHTML).imports;
    if (imports && HTMLScriptElement.supports?.(kImportmap)) {
      imports = { [kJsxImportSource]: imports[kJsxImportSource] };
    }
  } else {
    loader = loaders[el.type];
  }
  if (loader) {
    runScripts.push({ loader, code: el.innerHTML });
  }
});

runScripts.forEach(async (input) => {
  const murl = new URL(import.meta.url);
  const hash = await computeHash(JSON.stringify([murl, input, imports]));
  const cacheKey = "esm.sh/run/" + hash;
  let js = l.getItem(cacheKey);
  if (!js) {
    const res = await fetch(murl.origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const { transform } = await import(`./build`);
      const ret = await transform({ ...input, imports, hash });
      js = ret.code;
    }
    l.setItem(cacheKey, js!);
  }
  const script = d.createElement(kScript);
  script.type = "module";
  script.innerHTML = js!;
  d.body.appendChild(script);
});

async function computeHash(input: string): Promise<string> {
  const c = window.crypto;
  if (!c) {
    const { h64ToString } = await (await import(`./xxhash-wasm@1.0.2`))
      .default();
    return h64ToString(input);
  }
  const buffer = new Uint8Array(
    await c.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}
