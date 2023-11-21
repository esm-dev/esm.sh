/*! esm.sh run
 *
 * Add `<script type="module" src="https://esm.sh/run" defer></script>` to your HTML to run jsx/tsx in browser without build.
 *
 */

const d = document;
const runScripts: { loader: string; code: string }[] = [];
let jsxImportSource: string | undefined = undefined;

d.querySelectorAll("script").forEach((el) => {
  let loader: string | null = null;
  switch (el.type) {
    case "importmap": {
      const im = JSON.parse(el.innerHTML);
      jsxImportSource = im.imports?.["@jsxImportSource"];
      break;
    }
    case "text/babel":
    case "text/tsx":
      loader = "tsx";
      break;
    case "text/jsx":
      loader = "jsx";
      break;
    case "text/typescript":
    case "application/typescript":
      loader = "ts";
      break;
  }
  if (loader) {
    runScripts.push({ loader, code: el.innerHTML });
  }
});

runScripts.forEach(async (input) => {
  const murl = new URL(import.meta.url);
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(
        murl.pathname + input.loader + (jsxImportSource ?? "") +
          input.code,
      ),
    ),
  );
  const hash = [...buffer].map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  const cacheKey = "esm.sh/run/" + hash;
  let js = localStorage.getItem(cacheKey);
  if (!js) {
    const res = await fetch(murl.origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const { transform } = await import(`./build`);
      const ret = await transform({ ...input, jsxImportSource, hash });
      js = ret.code;
    }
    localStorage.setItem(cacheKey, js!);
  }
  const script = d.createElement("script");
  script.type = "module";
  script.innerHTML = js!;
  d.body.appendChild(script);
});
