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

export async function build(code: string): Promise<BuildOutput>;
export async function build(options: BuildInput): Promise<BuildOutput>;
export async function build(
  codeOrOptions: BuildInput | string,
): Promise<BuildOutput> {
  const options = typeof codeOrOptions === "string"
    ? { code: codeOrOptions }
    : codeOrOptions;
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

async function withCache(code: string): Promise<BuildOutput> {
  let key = code;
  if (globalThis.crypto && globalThis.crypto.subtle) {
    key = await hashText(code);
  }
  if (globalThis.localStorage) {
    const cached = localStorage.getItem(key);
    if (cached) {
      return JSON.parse(cached);
    }
  }
  const ret = await build(code);
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
