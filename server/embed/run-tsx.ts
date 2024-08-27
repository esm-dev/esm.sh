export async function tsx(
  filename: string,
  code: string,
  importMap: { imports?: Record<string, string> },
  target: string,
  id: string,
): Promise<Response> {
  let res = await fetch(urlFromCurrentModule(`/+${id}.mjs`));
  if (res.status === 404) {
    res = await fetch(urlFromCurrentModule("/transform"), {
      method: "POST",
      body: JSON.stringify({ filename, code, importMap, target }),
    });
    const ret: any = await res.json();
    if (ret.error) {
      throw new Error(ret.error.message);
    }
    res = new Response(ret.code, { headers: { "Content-Type": "application/javascript; charset=utf-8" } });
  }
  return res;
}

/** create a URL object from the given path in the current module. */
function urlFromCurrentModule(path: string) {
  return new URL(path, import.meta.url);
}
