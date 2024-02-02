
export interface ContentMap {
  contents?: Record<string, ContentSource>;
}

export interface ContentSource {
  url?: string;
  method?: string;
  token?: string;
  headers?: [string, string][] | Record<string, string>;
  payload?: any;
  timeout?: number;
  cacheTtl?: number;
  select?: string;
  stream?: boolean;
}
    // <use-content src="foo" map="this.bar" ssr></use-content>
    defineElement("use-content", (el) => {
      const src = attr(el, "src");
      if (!src) {
        return;
      }
      if (el.hasAttribute("store")) {
        return;
      }
      const cache = this.#contentCache;
      const render = (data: unknown) => {
        if (data instanceof Error) {
          el.innerHTML = createErrorTag(data[kMessage]);
          return;
        }
        const mapKey = attr(el, "mapKey");
        const content = toString(
          mapKey && !isNullish(data) ? (data as any)[mapKey] : data,
        );
        if (el.hasAttribute("html")) {
          el.innerHTML = content;
        } else {
          el.textContent = content;
        }
      };
      const load = () => {
        const renderedData = cache[src];
        if (renderedData) {
          if (renderedData instanceof Promise) {
            renderedData.then(render);
          } else {
            render(renderedData);
          }
        } else {
          cache[src] = fetch(this.basePath + "@hot-content", {
            method: "POST",
            body: stringify({ src, location: location.pathname }),
          }).then(async (res) => {
            if (res.ok) {
              const value = await res.json();
              cache[src] = value;
              return render(value);
            }
            let msg = res.statusText;
            const text = (await res.text()).trim();
            if (text) {
              msg = text;
              if (text.trimStart().startsWith("{")) {
                try {
                  const { error, message } = parse(text);
                  msg = error?.[kMessage] ?? message ?? msg;
                } catch (_) {
                  // ignore
                }
              }
            }
            delete cache[src];
            render(new Error(msg));
          });
        }
      };
      const liveProp = attr(el, "live");
      if (liveProp) {
        const live = parseInt(liveProp);
        if (live > 0) {
          const check = () => {
            delete cache[src];
            load();
          };
          setInterval(check, 1000 * live);
        }
      }
      load();
    });
