export type BuildOptions = {
  code: string;
  dependencies?: Record<string, string>;
  types?: string;
};

export type BuildResult = {
  id: string;
  url: string;
  bundleUrl: string;
};

export async function build(code :string): Promise<BuildResult>
export async function build(options: BuildOptions): Promise<BuildResult>
export async function build(codeOrOptions: BuildOptions|string): Promise<BuildResult> {
  const options = typeof codeOrOptions === "string" ? { code: codeOrOptions } : codeOrOptions;
  if (!options?.code) {
    throw new Error("esm.sh [build] <400> missing code");
  }
  const ret = await fetch("$ORIGIN/build", {
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

export default build;
