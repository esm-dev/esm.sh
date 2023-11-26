/*! esm.sh run
 *
 * Add `<script type="module" src="https://esm.sh/run"></script>` to run jsx/ts in browser without build step.
 *
 */

const d = document;
const l = localStorage;
const stringify = JSON.stringify;
const modUrl = new URL(import.meta.url);
const KB = 1024;
const kRun = "esm.sh/run";
const kScript = "script";
const kImportmap = "importmap";
const loaders = ["js", "jsx", "ts", "tsx", "babel"];
const imSupported = HTMLScriptElement.supports?.(kImportmap);
const imImports: Record<string, string> = {};
const runScripts: { loader: string; code: string }[] = [];

// lookup scripts
d.querySelectorAll(kScript).forEach((el) => {
  let loader: string | null = null;
  if (el.type === kImportmap) {
    const v = JSON.parse(el.innerHTML).imports;
    for (const k in v) {
      if (!imSupported || k === "@jsxImportSource") {
        imImports[k] = v[k];
      }
    }
  } else if (el.type.startsWith("text/")) {
    loader = el.type.slice(5);
    if (loaders.includes(loader)) {
      const code = el.innerHTML.trim();
      if (code.length > 100 * KB) {
        throw new Error(kRun + " " + code.length + " bytes exceeded limit.");
      }
      runScripts.push({ loader, code });
    }
  }
});

// transform and insert scripts
const imports = stringify(imImports);
runScripts.forEach(async (input, idx) => {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(
        input.loader + input.code + imports,
      ),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const jsCacheKey = kRun + ":" + idx;
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
