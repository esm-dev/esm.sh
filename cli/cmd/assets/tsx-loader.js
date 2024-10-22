import init, { transform } from "npm:esm-tsx@1.2.2";

const output = (data) => Deno.stdout.write(new TextEncoder().encode(JSON.stringify(data)));

try {
  await init();
  const [filename, code, importMap] = await (new Response(Deno.stdin.readable)).json();
  const imports = importMap?.imports;
  const ret = transform({
    filename,
    code,
    importMap,
    sourceMap: "inline",
    dev: {
      hmr: { runtime: "/@hmr" },
      refresh: imports?.react && !imports?.preact ? { runtime: "/@refresh" } : undefined,
      prefresh: imports?.preact ? { runtime: "/@prefresh" } : undefined,
    },
  });
  await output(ret);
} catch (err) {
  await output({ error: err.message });
}

Deno.exit();
