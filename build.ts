// deno-lint-ignore-file no-explicit-any

export type BuildInput = {
  code: string;
  loader?: "js" | "jsx" | "ts" | "tsx";
  dependencies?: Record<string, string>;
  types?: string;
};

export type BuildOutput = {
  id: string;
  url: string;
  bundleUrl: string;
};

export async function build(input: string | BuildInput): Promise<BuildOutput> {
  const options = typeof input === "string" ? { code: input } : input;
  if (!options?.code) {
    throw new Error("esm.sh [build] <400> missing code");
  }
  const ret: any = await fetch("$ORIGIN/build", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options),
  }).then((r) => r.json());
  if (ret.error) {
    throw new Error(
      `esm.sh [build] <${ret.error.status}> ${ret.error.message}`,
    );
  }
  return ret;
}

export async function transform(
  input: string | BuildInput & { target?: string },
): Promise<{ code: string }> {
  const options = typeof input === "string" ? { code: input } : input;
  Reflect.set(options, "transformOnly", true);
  return await build(options) as unknown as { code: string };
}

export async function esm<T extends object = Record<string, any>>(
  strings: TemplateStringsArray,
  ...values: any[]
): Promise<T & { _build: BuildOutput }> {
  const code = String.raw({ raw: strings }, ...values);
  const ret = await withCache(code);
  const mod: T = await import(ret.url);
  return {
    ...mod,
    _build: ret,
  };
}

export async function withCache(
  input: string | BuildInput,
): Promise<BuildOutput> {
  let key = typeof input === "string" ? input : JSON.stringify(input);
  if (globalThis.crypto) {
    key = await hashText(key);
  }
  if (globalThis.localStorage) {
    const cached = localStorage.getItem(key);
    if (cached) {
      return JSON.parse(cached);
    }
  }
  const ret = await build(input);
  if (globalThis.localStorage) {
    localStorage.setItem(key, JSON.stringify(ret));
  }
  return ret;
}

export async function hashText(s: string): Promise<string> {
  const buffer = await crypto.subtle.digest(
    "SHA-1",
    new TextEncoder().encode(s),
  );
  return Array.from(new Uint8Array(buffer)).map((b) =>
    b.toString(16).padStart(2, "0")
  ).join("");
}

export default build;
