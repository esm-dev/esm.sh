let jsxImportSource: string | undefined = undefined;
const runScripts: { loader: string; code: string }[] = [];

document.querySelectorAll("script").forEach((el) => {
  let loader: string | null = null;
  switch (el.type) {
    case "importmap": {
      const o = JSON.parse(el.innerHTML);
      jsxImportSource = o?.imports?.["@jsxImportSource"];
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
  const hash = await hashText(
    murl.pathname + input.loader + (jsxImportSource ?? "") +
      input.code,
  );
  let js = localStorage.getItem(hash);
  if (!js) {
    const res = await fetch(murl.origin + `/+${hash}.mjs`);
    if (res.ok) {
      js = await res.text();
    } else {
      const { transform } = await import(`./build`);
      const ret = await transform({ ...input, jsxImportSource, hash });
      js = ret.code;
    }
    localStorage.setItem(hash, js!);
  }
  const script = document.createElement("script");
  script.type = "module";
  script.innerHTML = js!;
  document.body.appendChild(script);
});

async function hashText(s: string): Promise<string> {
  const buffer = await crypto.subtle.digest(
    "SHA-1",
    new TextEncoder().encode(s),
  );
  return Array.from(new Uint8Array(buffer)).map((b) =>
    b.toString(16).padStart(2, "0")
  ).join("");
}
