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
    return new Response(
      (await Deno.open(new URL("../dist" + url.pathname, import.meta.url)))
        .readable,
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
