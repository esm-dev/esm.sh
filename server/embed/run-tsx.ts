const stringify = JSON.stringify;

export async function tsx(
  url: URL,
  code: string,
  importMap: { imports?: Record<string, string> },
  target: string,
  cachePromise: Promise<Cache>,
): Promise<Response> {
  const filename = url.pathname.split("/").pop()!;
  const extname = filename.split(".").pop()!;
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(extname + code + stringify(importMap) + target + "false"),
    ),
  );
  const id = [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
  const cache = await cachePromise;
  const cacheKey = new URL(url);
  cacheKey.searchParams.set("_tsxid", id);

  let res = await cache.match(cacheKey);
  if (res) {
    return res;
  }

  res = await fetch(urlFromCurrentModule(`/+${id}.mjs`));
  if (res.status === 404) {
    res = await fetch(urlFromCurrentModule("/transform"), {
      method: "POST",
      body: stringify({ filename, code, importMap, target }),
    });
    const ret = await res.json();
    if (ret.error) {
      throw new Error(ret.error.message);
    }
    res = new Response(ret.code, { headers: { "Content-Type": "application/javascript; charset=utf-8" } });
  }
  if (!res.ok) {
    return res;
  }

  cache.put(cacheKey, res.clone());
  return res;
}

/** create a URL object from the given path in the current module. */
function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
}
