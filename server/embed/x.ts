/*!
 * ⚡️ esm.sh/x - ts/jsx/vue/svelte just works™️ in browser.
 * Usage: <script type="module" src="app.tsx"> → <script src="https://esm.sh/x" href="app.tsx">
 */

((document, location) => {
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const $: typeof document.querySelector = (s: string) => document.querySelector(s);
  if (location.protocol == "file:" || ["localhost", "127.0.0.1"].includes(location.hostname)) {
    console.error("[esm.sh/x] Please start your app with `npx esm.sh serve` in development env.");
    return;
  }
  let main = currentScript?.getAttribute("href");
  if (main) {
    const mainUrl = new URL(main, location.href);
    const ctx = btoa(location.pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    const v = $<HTMLMetaElement>("meta[name=version]")?.content;
    const { searchParams, pathname } = mainUrl;
    if (pathname.endsWith("/uno.css")) {
      searchParams.set("ctx", ctx);
    } else if ($("script[type=importmap]")) {
      searchParams.set("im", ctx);
    }
    if (v) {
      searchParams.set("v", v);
    }
    main = new URL(currentScript!.src).origin + "/" + mainUrl;
    if (pathname.endsWith(".css")) {
      currentScript!.insertAdjacentHTML("afterend", `<link rel="stylesheet" href="${main}">`);
    } else {
      import(main);
    }
  }
})(document, location);
