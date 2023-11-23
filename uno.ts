/*! esm.sh uno
 *
 * Add `<script type="module" src="https://esm.sh/uno" defer></script>` to your HTML to enable UnoCSS in browser without setup.
 *
 */

const UNO_VERSION = "0.57.7";
const kUnoSegment = "/@unocss/";

(async (d: Document, l: Storage) => {
  const head = d.head;
  const murl = new URL(import.meta.url);
  const query = new URLSearchParams(location.search);
  const input = d.body.innerHTML;

  let css: string | null = null;
  if (query.has("dev")) {
    const [{ UnoGenerator }, { default: presetWind }] = await Promise.all([
      import(`.${kUnoSegment}core@${UNO_VERSION}`),
      import(`.${kUnoSegment}preset-wind@${UNO_VERSION}?bundle`),
    ]);
    const uno = new UnoGenerator({ presets: [presetWind()] });
    const ret = await uno.generate(input);
    if (ret.matched.size > 0) {
      css = ret.css;
    }
  } else {
    const hash = await computeHash(input);
    const cacheKey = "esm.sh/uno/" + hash;
    css = l.getItem(cacheKey);
    if (!css) {
      const res = await fetch(murl.origin + `/+${hash}.css`);
      if (res.ok) {
        css = await res.text();
      } else {
        const res = await fetch(murl.origin + "/uno-generate/" + hash, {
          method: "POST",
          body: input,
        });
        if (!res.ok) throw new Error(res.statusText);
        const ret = await res.json();
        css = ret.css;
      }
      l.setItem(cacheKey, css ?? "/* empty */");
    }
  }

  if (css) {
    const link = d.createElement("link");
    const style = d.createElement("style");
    link.rel = "stylesheet";
    link.href = `https://esm.sh${kUnoSegment}reset@${UNO_VERSION}/tailwind.css`;
    style.textContent = css;
    head.appendChild(link);
    head.appendChild(style);
  }
})(document, localStorage);

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
