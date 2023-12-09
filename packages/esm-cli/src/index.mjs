import { openFile } from "./fs.mjs";

const enc = new TextEncoder();

/**
 * Creates a fetch handler for serving hot applications.
 * @param {import("../types").ServeOptions} options
 * @returns {(req: Request) => Promise<Response>}
 */
export const serveHot = (options) => {
  const { root = ".", fallback = "index.html", watch } = options;
  const fsWatchHandlers = new Set();
  if (watch) {
    import("node:fs").then(({ watch }) => {
      watch(
        root,
        { recursive: true },
        (event, filename) => {
          if (!/(^|\/)(\.|node_modules\/)/.test(filename) && !filename.endsWith(".log")) {
            fsWatchHandlers.forEach((handler) =>
              handler(event === "change" ? "modify" : event, "/" + filename)
            );
          }
        },
      );
      console.log(`Watching files changed...`);
    });
  }
  return async (req) => {
    const url = new URL(req.url);
    const pathname = decodeURIComponent(url.pathname);
    if (watch && pathname === "/hot-notify") {
      let handler;
      return new Response(
        new ReadableStream({
          start(controller) {
            const enqueue = (chunk) => controller.enqueue(chunk);
            handler = (type, name) => {
              enqueue(enc.encode("event: fs-notify\ndata: "));
              enqueue(enc.encode(JSON.stringify({ type, name })));
              enqueue(enc.encode("\n\n"));
            };
            fsWatchHandlers.add(handler);
            enqueue(enc.encode(": hot notify stream\n\n"));
          },
          cancel() {
            handler && fsWatchHandlers.delete(handler);
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
    let file = pathname.includes(".") ? await openFile(root + pathname) : null;
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
      const list = [
        pathname + ".html",
        pathname + "/index.html",
        "/404.html",
        "/" + fallback,
      ];
      for (const filename of list) {
        file = await openFile(root + filename);
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
                file.close();
                controller.close();
                return;
              }
              controller.enqueue(new Uint8Array(value));
              pump();
            };
            pump();
          },
        }),
        { headers },
      );
    }
    return new Response("Not Found", { status: 404 });
  };
};
