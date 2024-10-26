/*! 🚀 esm.sh/run - ts/jsx just works™️ in browser. */

((document) => {
  const $ = document.querySelector;
  const currentScript = document.currentScript as HTMLScriptElement | null;
  const modUrl = currentScript?.src || import.meta.url;
  const { hostname, href, pathname } = location;

  // import the `main` module from esm.sh if it's provided.
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.tsx"></script>
  const el = currentScript ?? $<HTMLScriptElement>("script[type=module][src='" + modUrl + "'][main]");
  if (el) {
    const main = el.getAttribute("main");
    if (main) {
      if (hostname === "localhost" || hostname === "127.0.0.1" || /^192\.168\.\d+\.\d+$/.test(hostname)) {
        alert("Please serve your app with `esm.sh run` for local development.");
        return;
      }
      const mainUrl = new URL(main, href);
      const q = mainUrl.searchParams;
      const v = $<HTMLMetaElement>("meta[name=version]")?.content;
      mainUrl.search = "";
      if ($("script[type=importmap]")) {
        q.set(
          "im",
          (HTMLScriptElement.supports("importmap") ? "y" : "N")
            + btoa(pathname).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, ""),
        );
      }
      if (v) {
        q.set("v", v);
      }
      import(new URL(modUrl).origin + "/" + mainUrl);
    }
  }

  // compatibility with esm.sh/run(v1) which has been renamed to 'esm.sh/tsx'
  if ($<HTMLScriptElement>("script[type^='text/']")) {
    import(new URL("/tsx", modUrl).href);
  }
})(document);
