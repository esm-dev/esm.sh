import { openFile } from "./fs.mjs";

const enc = new TextEncoder();
const regexpPluginNaming = /^[a-zA-Z0-9][\w\.\-]*(@\d+\.\d+\.\d+)?$/;

/**
 * serves a hot app.
 * @param {import("../types").ServeOptions} options
 * @returns {(req: Request) => Promise<Response>}
 */
export const serveHot = (options) => {
  if (options.plugins) {
    options.plugins = options.plugins.filter((name) =>
      regexpPluginNaming.test(name)
    );
  }
  const { spa, watch, plugins, cwd = "." } = options;
  const fsWatchHandlers = new Set();
  if (watch) {
    import("node:fs").then(({ watch }) => {
      watch(
        cwd,
        { recursive: true },
        (event, filename) => {
          fsWatchHandlers.forEach((handler) =>
            handler(event === "change" ? "modify" : event, "/" + filename)
          );
        },
      );
      console.log(`Watching files changed...`);
    });
  }
  if (plugins?.length) {
    console.log(`Using plugins: ${plugins.join(", ")}`);
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
    let file = pathname.includes(".") ? await openFile(cwd + pathname) : null;
    if (!file && pathname === "/sw.js") {
      const hotUrl = new URL("https://esm.sh/v135/hot");
      if (plugins?.length) {
        hotUrl.searchParams.set("plugins", plugins);
      }
      return new Response(`import hot from "${hotUrl.href}";hot.listen();`, {
        headers: {
          "content-type": "application/javascript; charset=utf-8",
          "last-modified": new Date().toUTCString(),
        },
      });
    }
    if (!file) {
      if (spa) {
        const index = "index.html";
        if (typeof spa === "string" && spa.endsWith(".html")) {
          index = spa;
        } else if (spa.index && spa.index.endsWith(".html")) {
          index = spa.index;
        }
        file = await openFile(cwd + "/" + index);
      } else {
        file = await openFile(cwd + pathname + ".html");
        if (!file) {
          file = await openFile(cwd + pathname + "/index.html");
        }
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

export default serveHot;
