/*! 🚀 esm.sh/run - ts/jsx just works™️ in browser. */

((modUrl, $) => {
  // import the `main` module from esm.sh if it's provided.
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.tsx"></script>
  const el = $<HTMLScriptElement>("script[type=module][src][main]");
  if (el) {
    const src = el.src;
    const main = el.getAttribute("main");
    const { hostname, href, pathname, search } = location;
    if (src === modUrl && main) {
      if (hostname === "localhost" || hostname === "127.0.0.1" || /^192\.168\.\d+\.\d+$/.test(hostname)) {
        fetch(main).then((res) => {
          if (res.ok) {
            if (res.headers.get("server") === "esm.sh/run") {
              import(main);
            } else {
              alert("Please serve your app with `npx esm.sh run` for local development.");
            }
          }
        });
      } else {
        const mainUrl = new URL(main, href);
        const q = mainUrl.searchParams;
        const v = q.get("v") || q.get("version");
        mainUrl.search = "";
        if ($("script[type=importmap]")) {
          q.set(
            "im",
            (HTMLScriptElement.supports("importmap") ? "y" : "N") +
              btoa(pathname + search).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, ""),
          );
        }
        if (v) {
          q.set("v", v);
        }
        import(new URL(src).origin + "/" + mainUrl);
      }
    }
  }

  // compatibility with esm.sh/run(v1) which has been renamed to 'esm.sh/tsx'
  if ($<HTMLScriptElement>("script[type^='text/']")) {
    import(new URL("/tsx", modUrl).href);
  }
})(import.meta.url, document.querySelector);
