Deno.serve((req) => {
  const url = new URL(req.url);
  if (url.pathname === "/") {
    return new Response(Deno.readFileSync("index.html"),{
      headers: new Headers({
        "content-type": "text/html",
        "cache-control": "public, max-age=0, revalidate",
      })
    });
  }
  const ext = url.pathname.split(".").pop();
  try {
    return new Response(Deno.readFileSync(url.pathname.slice(1)),{
      headers: new Headers({
        "content-type": ext === "js" ? "application/javascript" : "text/css",
        "cache-control": "public, max-age=0, revalidate",
      }),
    });
  } catch (e) {
    return new Response("Not found",{
      status: 404,
    });
  }
});
