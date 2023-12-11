import { openFile } from "./fs.mjs";

const enc = new TextEncoder();
const fsFilter = (filename) => {
  return !/(^|\/)(\.|node_modules\/)/.test(filename) &&
    !filename.endsWith(".log");
};

/**
 * Creates a fetch handler for serving hot applications.
 * @param {import("../types").ServeOptions} options
 * @returns {(req: Request) => Promise<Response>}
 */
export const serveHot = (options) => {
  const { root = ".", fallback = "index.html" } = options;

  const watchCallbacks = new Set();
  const watchFS = async () => {
    const { watch } = await import("node:fs");
    watch(root, { recursive: true }, (event, filename) => {
      if (fsFilter(filename)) {
        watchCallbacks.forEach((handler) =>
          handler(event === "change" ? "modify" : event, "/" + filename)
        );
      }
    });
    console.log(`Watching files changed...`);
  };
  watchFS.watched = false;

  const ls = async (dir, pos) => {
    const { readdir } = await import("node:fs/promises");
    const files = [];
    const list = await readdir(dir, { withFileTypes: true });
    for (const entry of list) {
      const name = [pos, entry.name].filter(Boolean).join("/");
      if (entry.isDirectory()) {
        files.push(...(await ls(dir + "/" + entry.name, name)));
      } else if (fsFilter(name)) {
        files.push(name);
      }
    }
    return files;
  };

  return async (req) => {
    const url = new URL(req.url);
    const pathname = decodeURIComponent(url.pathname);

    if (pathname === "/hot-notify") {
      if (!watchFS.watched) {
        watchFS.watched = true;
        await watchFS();
      }
      let notify;
      return new Response(
        new ReadableStream({
          start(controller) {
            const enqueue = (chunk) => controller.enqueue(chunk);
            notify = (type, name) => {
              enqueue(enc.encode("event: fs-notify\ndata: "));
              enqueue(enc.encode(JSON.stringify({ type, name })));
              enqueue(enc.encode("\n\n"));
            };
            watchCallbacks.add(notify);
            enqueue(enc.encode(": hot notify stream\n\n"));
          },
          cancel() {
            notify && watchCallbacks.delete(notify);
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

    if (pathname === "/hot-index") {
      const entries = await ls(root);
      return Response.json(entries);
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
