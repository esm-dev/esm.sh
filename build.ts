export type BuildInput = {
  code: string;
  loader?: "js" | "jsx" | "ts" | "tsx";
  dependencies?: Record<string, string>;
  types?: string;
};

export type TransformOptions = {
  target?:
    | "deno"
    | "denonext"
    | "node"
    | "esnext"
    | `es201${5 | 6 | 7 | 8 | 9}`
    | `es202${0 | 1 | 2}`;
  imports?: Record<string, string>;
};

export type BuildOutput = {
  id: string;
  url: string;
  bundleUrl: string;
};

export async function build(input: string | BuildInput): Promise<BuildOutput> {
  const options = typeof input === "string" ? { code: input } : input;
  if (!options.code) {
    throw new Error("esm.sh [build] <400> missing code");
  }
  const ret = await fetch(new URL("/build", import.meta.url), {
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
  input: string | (BuildInput & TransformOptions),
): Promise<{ code: string }> {
  const options = typeof input === "string" ? { code: input } : input;
  if (!options.code) {
    throw new Error("esm.sh [transform] <400> missing code");
  }
  const loader = options.loader || "tsx";
  const imports = JSON.stringify(options.imports || {});
  const hash = await computeHash(loader + options.code + imports);
  options.loader = loader;
  Reflect.set(options, "imports", imports);
  Reflect.set(options, "hash", hash);
  Reflect.set(options, "transformOnly", true);
  const res = await fetch(new URL(`/+${hash}.mjs`, import.meta.url));
  if (res.ok) {
    return { code: await res.text() };
  } else {
    return await build(options) as unknown as { code: string };
  }
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

async function withCache(
  input: string | BuildInput,
): Promise<BuildOutput> {
  const key = await computeHash(
    typeof input === "string" ? input : JSON.stringify(input),
  );
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

async function computeHash(input: string): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export default build;
