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
  try {
    const fileUrl = new URL("../dist" + url.pathname, import.meta.url);
    let body = (await Deno.open(fileUrl)).readable;
    if (url.pathname === "/lsp/typescript/worker.js") {
      let replaced = false;
      body = body.pipeThrough(
        new TransformStream({
          transform: async (chunk, controller) => {
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
      "content-type": fileUrl.pathname.endsWith(".css")
        ? "text/css"
        : "application/javascript",
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
});
