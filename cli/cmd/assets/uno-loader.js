import { generate } from "npm:esm-unocss@0.4.1";

const output = (data) => Deno.stdout.write(new TextEncoder().encode(JSON.stringify(data)));

try {
  const [configCSS, data] = await (new Response(Deno.stdin.readable)).json();
  const { css } = await generate(data, configCSS ? { configCSS } : undefined);
  await output({ code: css });
} catch (err) {
  await output({ error: err.message });
}

Deno.exit();
