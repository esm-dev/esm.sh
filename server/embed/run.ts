/*! 🔥 esm.sh/run - ts/jsx just works™️ in browser. */

function setup() {
  const { querySelector } = document;
  const importEsmScript = (name: string) => import(new URL("/" + name, import.meta.url).toString());

  // import the `main` module from esm.sh if it's provided.
  // e.g. <script type="module" src="https://esm.sh/run" main="/main.tsx"></script>
  const el = querySelector<HTMLScriptElement>("script[type='module'][src][main]");
  if (el) {
    const src = el.src;
    const main = el.getAttribute("main");
    if (src === import.meta.url && main) {
      const { hostname, href } = location;
      if (hostname === "localhost" || hostname === "127.0.0.1") {
        fetch(main).then((res) => {
          res.body?.cancel();
          if (res.ok) {
            if (/^(text|application)\/javascript/i.test(res.headers.get("content-type")!)) {
              import(main);
            } else {
              importEsmScript("run-helper");
            }
          }
        });
      } else {
        // import https://esm.sh/[main]
        import(new URL(src).origin + "/" + new URL(main, href).toString());
      }
    }
  }

  // compatibility with esm.sh/run(v1) which has been renamed to 'esm.sh/tsx'
  if (querySelector<HTMLScriptElement>("script[type^='text/']")) {
    importEsmScript("tsx");
  }
}

setup();
