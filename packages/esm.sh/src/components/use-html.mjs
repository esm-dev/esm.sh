 // <use-html src="./pages/foo.html" ssr></use-html>
    // <use-html src="./blog/foo.md" ssr></use-html>
    // <use-html src="./icons/foo.svg" ssr></use-html>
    defineElement("use-html", (el) => {
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      const root = el.hasAttribute("shadow")
        ? el.attachShadow({ mode: "open" })
        : el;
      const url = new URL(src, loc.href);
      const { pathname, searchParams } = url;
      if ([".md", ".markdown"].some((ext) => pathname.endsWith(ext))) {
        searchParams.set("html", "");
      }
      const load = async (hmr?: boolean) => {
        if (hmr) {
          addTimeStamp(url);
        }
        const res = await fetch(url);
        const text = await res.text();
        root.innerHTML = res.ok ? text : createErrorTag(text);
      };
      if (isDev && isSameOrigin(url)) {
        __hot_hmr_callbacks.add(pathname, () => load(true));
      }
      load();
    });

