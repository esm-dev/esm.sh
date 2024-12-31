/*!
 * ⚡️ esm.sh/x - ts/jsx/vue/svelte just works™️ in browser.
 * Usage: <script src="app.tsx"> → <script src="https://esm.sh/x" href="app.tsx">
 */

((document, location) => {
  const { hostname } = location;
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const $: typeof document.querySelector = (s: string) => document.querySelector(s);
  if (hostname == "localhost" || hostname == "127.0.0.1" || /^192\.168\.\d+\.\d+$/.test(hostname)) {
    console.error("[esm.sh/x] Please start your app with `npx esm.sh serve` in development env.");
    return;
  }
  let main = currentScript?.getAttribute("href");
  if (main) {
    const mainUrl = new URL(main, location.href);
    const { searchParams, pathname } = mainUrl;
    const ctx = btoa(location.pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    const v = $<HTMLMetaElement>("meta[name=version]")?.content;
    if (pathname == "/uno.css") {
      searchParams.set("ctx", ctx);
    } else if ($("script[type=importmap]")) {
      searchParams.set("im", ctx);
    }
    if (v) {
      searchParams.set("v", v);
    }
    main = new URL(currentScript!.src).origin + "/" + mainUrl;
    if (pathname.endsWith(".css")) {
      document.write(`<link rel="stylesheet" href="${main}">`);
    } else {
      import(main);
    }
  }
})(document, location);
