export function RPC(modUrl, fnName) {
  const { pathname, searchParams } = new URL(modUrl);
  const url = new URL("/@rpc" + pathname, modUrl);
  const im = searchParams.get("im");
  if (im) {
    url.searchParams.set("im", im);
  }
  url.searchParams.set("fn", fnName);
  return async (...args) => {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(args),
    });
    if (!res.ok) {
      await res.body?.cancel();
      throw new Error(`failed to call RPC(${pathname}.${fnName}): ${res.status} ${res.statusText}`);
    }
    const ret = await res.json();
    if (ret.error) {
      throw new Error(`failed to call RPC(${pathname}.${fnName}): ${ret.error}`);
    }
    return ret.result;
  };
}

export function proxy(modUrl) {
  return new Proxy(Object.create(null), { get: (_, prop) => RPC(modUrl, prop) });
}
