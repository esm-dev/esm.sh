import { renderToString } from "../dist/index.js";

const appTsx = `import confetti from "https://esm.sh/canvas-confetti@1.6.0"
import { useEffect } from "react"
import { message } from "./greeting.ts"

export default function App() {
  useEffect(() => {
    confetti()
    log(message)
  }, [])
  return <h1>{message}</h1>;
}
`;

async function serveDist(url, req, notFound) {
  if (url.pathname === "/") {
    return notFound(url, req);
  }
  if (url.pathname.endsWith("/")) {
    return new Response("Directory listing not supported", {
      status: 400,
    });
  }
  try {
    const fileUrl = new URL("../dist" + url.pathname, import.meta.url);
    let body = (await Deno.open(fileUrl)).readable;
    if (url.pathname === "/lsp/typescript/worker.js") {
      let replaced = false;
      body = body.pipeThrough(
        new TransformStream({
          transform: (chunk, controller) => {
            if (replaced) {
              controller.enqueue(chunk);
              return;
            }
            const text = new TextDecoder().decode(chunk);
            if (text.includes('from "typescript"')) {
              controller.enqueue(new TextEncoder().encode(
                text.replace(
                  'from "typescript"',
                  'from "https://esm.sh/typescript@5.3.3?bundle"',
                ),
              ));
              replaced = true;
            } else {
              controller.enqueue(chunk);
            }
          },
        }),
      );
    }
    const headers = new Headers({
      "transfer-encoding": "chunked",
      "cache-control": "public, max-age=0, revalidate",
      "content-type": getContentType(fileUrl.pathname),
    });
    return new Response(body, { headers });
  } catch (e) {
    if (e instanceof Deno.errors.NotFound) {
      return notFound(url, req);
    }
    return new Response(e.message, {
      status: 500,
    });
  }
}

async function serveCWD(url, req) {
  const filename = url.pathname.slice(1) || "index.html";
  try {
    const fileUrl = new URL(filename, import.meta.url);
    let body = (await Deno.open(fileUrl)).readable;
    if (filename === "lazy-ssr.html") {
      let replaced = false;
      const ssrOutput = await renderToString({
        filename: "App.tsx",
        code: appTsx,
        padding: {
          top: 8,
          bottom: 8,
        },
        userAgent: req.headers.get("user-agent"),
        fontMaxDigitWidth: 7.22,
      });
      body = body.pipeThrough(
        new TransformStream({
          transform: async (chunk, controller) => {
            if (replaced) {
              controller.enqueue(chunk);
              return;
            }
            const text = new TextDecoder().decode(chunk);
            const searchExpr = /\{SSR}/;
            const m = text.match(searchExpr);
            if (m) {
              controller.enqueue(new TextEncoder().encode(
                text.replace(searchExpr, ssrOutput),
              ));
              replaced = true;
            } else {
              controller.enqueue(chunk);
            }
          },
        }),
      );
    }
    const headers = new Headers({
      "transfer-encoding": "chunked",
      "cache-control": "public, max-age=0, revalidate",
      "content-type": getContentType(fileUrl.pathname),
    });
    return new Response(body, { headers });
  } catch (e) {
    if (e instanceof Deno.errors.NotFound) {
      return new Response("Not found", {
        status: 404,
      });
    }
    return new Response(e.message, {
      status: 500,
    });
  }
}

function getContentType(pathname) {
  if (pathname.endsWith(".css")) {
    return "text/css; utf-8";
  }
  if (pathname.endsWith(".js")) {
    return "application/javascript; utf-8";
  }
  if (pathname.endsWith(".html")) {
    return "text/html; utf-8";
  }
  return "application/octet-stream";
}

Deno.serve(async (req) => {
  let url = new URL(req.url);
  if (url.pathname.startsWith("/dist/")) {
    url = new URL(url.pathname.slice(5), url);
  }
  return serveDist(url, req, serveCWD);
});
