/*! 🎨 esm.sh/uno - UnoCSS engine as a CDN. */

((document) => {
  const { hostname, pathname, origin } = location;
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const btoaUrl = (url: string) => btoa(url).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  if (currentScript) {
    if (hostname === "localhost" || hostname === "127.0.0.1" || /^192\.168\.\d+\.\d+$/.test(hostname)) {
      alert("Please serve your app with `esm.sh run` for local development.");
      return;
    }
    const unocssUrl = new URL("/uno.css", currentScript.src);
    const q = unocssUrl.searchParams;
    const v = document.querySelector<HTMLMetaElement>("meta[name=version]")?.content;
    q.set("ctx", btoaUrl(origin + pathname));
    if (v) {
      q.set("v", v);
    }
    document.write(`<link rel="stylesheet" href="${unocssUrl}">`);
  }
})(document);
