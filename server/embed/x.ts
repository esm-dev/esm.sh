/*!
 * ⚡️ esm.sh/x - ts/jsx/vue/svelte just works™️ in browser.
 * Usage: <script type="module" src="app.tsx"> → <script src="https://esm.sh/x" href="app.tsx">
 */

((document, location) => {
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const src = currentScript?.src;
  const href = currentScript?.getAttribute("href");
  if (location.protocol === "file:" || /^(([\w\-]+\.)?local(host)?|127\.0\.0\.1)$/.test(location.hostname)) {
    console.error("[esm.sh/x] Please start your app with `esm.sh dev` in development mode.");
    return;
  }
  if (src && href) {
    const cdnOrigin = new URL(src).origin;
    const mainUrl = new URL(href, location.href);
    const searchParams = mainUrl.searchParams;
    const isCSS = mainUrl.pathname.endsWith(".css");
    const ctx = btoa(location.pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    const version = document.querySelector<HTMLMetaElement>("meta[name=version]")?.content;
    if (isCSS) {
      searchParams.set("ctx", ctx);
    } else if (document.querySelector("script[type=importmap]")) {
      searchParams.set("im", ctx);
    }
    if (version && /^[\w\-.]+$/.test(version)) {
      searchParams.set("v", version);
    }
    if (isCSS) {
      const style = document.createElement("style");
      const link = document.createElement("link");
      style.textContent = "body{visibility:hidden}";
      link.rel = "stylesheet";
      link.href = cdnOrigin + "/" + mainUrl;
      link.onload = () => style.remove();
      currentScript.after(style, link);
    } else {
      import(cdnOrigin + "/" + mainUrl);
    }
  }
})(document, location);
