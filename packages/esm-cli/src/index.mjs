import fs from "./fs.mjs";
import { enc, globToRegExp } from "./util.mjs";

/**
 * Creates a fetch handler for serving hot applications.
 * @param {import("../types").ServeOptions} options
 * @returns {(req: Request) => Promise<Response>}
 */
export const serveHot = (options) => {
  const { root = ".", fallback = "index.html" } = options;
  const w = fs.watch(root);

  return async (req) => {
    const url = new URL(req.url);
    const pathname = decodeURIComponent(url.pathname);

    if (pathname === "/@hot-notify") {
      let dispose;
      return new Response(
        new ReadableStream({
          start(controller) {
            const enqueue = (chunk) => controller.enqueue(chunk);
            dispose = w((type, name) => {
              enqueue(enc.encode("event: fs-notify\ndata: "));
              enqueue(enc.encode(JSON.stringify({ type, name })));
              enqueue(enc.encode("\n\n"));
            });
            enqueue(enc.encode(": hot notify stream\n\n"));
          },
          cancel() {
            dispose?.();
          },
        }),
        {
          headers: {
            "transfer-encoding": "chunked",
            "content-type": "text/event-stream",
          },
        },
      );
    }

    if (pathname === "/@hot-index") {
      const entries = await fs.ls(root);
      return Response.json(entries);
    }

    if (pathname === "/@hot-glob") {
      const headers = new Headers({
        "content-type": "hot/glob",
        "content-index": "2",
      });
      const glob = url.searchParams.get("pattern");
      if (!glob) {
        return new Response("[]", { headers });
      }
      try {
        const entries = await fs.ls(root);
        const matched = entries.filter((entry) =>
          glob.includes(entry) || entry.match(globToRegExp(glob))
        );
        if (!matched.length) {
          return new Response("[]", { headers });
        }
        const names = enc.encode(JSON.stringify(matched) + "\n");
        const sizes = await Promise.all(matched.map(async (filename) => {
          const stat = await fs.stat(root + "/" + filename);
          return stat.size;
        }));
        headers.set("content-index", [names.length, ...sizes].join(","));
        let currentFile;
        return new Response(
          new ReadableStream({
            start(controller) {
              const enqueue = (chunk) => controller.enqueue(chunk);
              const pipe = async () => {
                const filename = matched.shift();
                if (!filename) {
                  controller.close();
                  return;
                }
                currentFile = await fs.open(root + "/" + filename);
                const reader = currentFile.body.getReader();
                const pump = async () => {
                  const { done, value } = await reader.read();
                  if (done) {
                    currentFile.close();
                    pipe();
                    return;
                  }
                  enqueue(new Uint8Array(value));
                  pump();
                };
                pump();
              };
              enqueue(names);
              pipe();
            },
            cancel() {
              currentFile?.close();
            },
          }),
          { headers },
        );
      } catch (e) {
        return new Response(e.message, { status: 500 });
      }
    }

    let file = pathname.includes(".") ? await fs.open(root + pathname) : null;
    if (!file && pathname === "/sw.js") {
      const hotUrl = new URL("https://esm.sh/v135/hot");
      return new Response(`import hot from "${hotUrl.href}";hot.listen();`, {
        headers: {
          "content-type": "application/javascript; charset=utf-8",
          "last-modified": new Date().toUTCString(),
        },
      });
    }
    if (!file) {
      switch (pathname) {
        case "/apple-touch-icon-precomposed.png":
        case "/apple-touch-icon.png":
        case "/robots.txt":
        case "/favicon.ico":
          return new Response("Not found", { status: 404 });
      }
      const list = [
        pathname + ".html",
        pathname + "/index.html",
        "/404.html",
        "/" + fallback,
      ];
      for (const filename of list) {
        file = await fs.open(root + filename);
        if (file) break;
      }
    }
    if (file) {
      const headers = new Headers({
        "transfer-encoding": "chunked",
        "content-type": file.contentType,
        "content-length": file.size.toString(),
      });
      if (file.lastModified) {
        headers.set("last-modified", new Date(file.lastModified).toUTCString());
      }
      return new Response(
        new ReadableStream({
          start(controller) {
            const reader = file.body.getReader();
            const pump = async () => {
              const { done, value } = await reader.read();
              if (done) {
                controller.close();
                file.close();
                return;
              }
              controller.enqueue(new Uint8Array(value));
              pump();
            };
            pump();
          },
          cancel() {
            file.close();
          },
        }),
        { headers },
      );
    }
    return new Response("Not Found", { status: 404 });
  };
};
