/*! 🚀 esm.sh/x - ts/jsx/vue/svelte just works™️ in browser. */

((document) => {
  const $: typeof document.querySelector = (s: string) => document.querySelector(s);
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const modUrl = currentScript?.src || import.meta.url;
  const { hostname, href, pathname, origin } = location;

  // import the `main` module from esm.sh if it's provided.
  // e.g. <script type="module" src="https://esm.sh/x" main="/main.tsx"></script>
  const el = currentScript ?? $<HTMLScriptElement>("script[type=module][main][src='" + modUrl + "']");
  if (el) {
    const main = el.getAttribute("main");
    if (main) {
      if (hostname === "localhost" || hostname === "127.0.0.1" || /^192\.168\.\d+\.\d+$/.test(hostname)) {
        console.error("[esm.sh/x] Please serve your app with `esm.sh serve` in development mode.");
        return;
      }
      const mainUrl = new URL(main, href);
      const q = mainUrl.searchParams;
      const v = $<HTMLMetaElement>("meta[name=version]")?.content;
      if ($("script[type=importmap]")) {
        q.set("im", btoa(pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""));
      }
      if (v) {
        q.set("v", v);
      }
      import(mainUrl.origin === origin ? new URL(modUrl).origin + "/" + mainUrl : "" + mainUrl);
    }
  }
})(document);
