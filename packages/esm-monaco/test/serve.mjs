Deno.serve(async (req) => {
  const url = new URL(req.url);
  if (url.pathname === "/") {
    return new Response(
      (await Deno.open(new URL("index.html", import.meta.url))).readable,
      {
        headers: new Headers({
          "transfer-encoding": "chunked",
          "content-type": "text/html",
          "cache-control": "public, max-age=0, revalidate",
        }),
      },
    );
  }
  const ext = url.pathname.split(".").pop();
  try {
    let body =
      (await Deno.open(new URL("../dist" + url.pathname, import.meta.url)))
        .readable;
    if (url.pathname === "/lsp/typescript/worker.js") {
      const ts = new TransformStream({
        transform: async (chunk, controller) => {
          const text = new TextDecoder().decode(chunk);
          if (/from"typescript"/.test(text)) {
            controller.enqueue(
              new TextEncoder().encode(
                text.replace(
                  /from"typescript"/,
                  'from"https://esm.sh/typescript@5.3.3?bundle"',
                ),
              ),
            );
          } else {
            controller.enqueue(chunk);
          }
        },
      });
      body = body.pipeThrough(ts);
    }
    return new Response(
      body,
      {
        headers: new Headers({
          "content-type": ext === "js" ? "application/javascript" : "text/css",
          "cache-control": "public, max-age=0, revalidate",
        }),
      },
    );
  } catch (e) {
    return new Response("Not found", {
      status: 404,
    });
  }
});
