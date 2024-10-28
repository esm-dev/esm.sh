import { TextLineStream } from "jsr:@std/streams@1.0.7/text-line-stream";

const enc = new TextEncoder();
const output = (data, type = ">") => Deno.stdout.write(enc.encode(">>" + type + JSON.stringify(data) + "\n"));
const error = (message) => output(message, "!");

let uno;
async function unocss(configCSS, data) {
  if (!uno || uno.configCSS !== configCSS) {
    uno = import("npm:esm-unocss@0.8.0").then(({ init }) => init(configCSS));
    uno.configCSS = configCSS;
  }
  const { update, generate } = await uno;
  await update(data);
  return await generate();
}

let esmTsx;
async function tsx(filename, importMap) {
  if (!esmTsx) {
    esmTsx = import("npm:esm-tsx@1.2.5").then(async ({ init, transform }) => {
      await init();
      return { transform };
    });
  }
  const { transform } = await esmTsx;
  const imports = importMap?.imports;
  if (imports) {
    for (const [specifier, resolved] of Object.entries(imports)) {
      if (
        (specifier === "react-dom" || specifier.startsWith("react-dom/"))
        && (resolved.startsWith("https://") || resolved.startsWith("http://"))
      ) {
        const url = new URL(resolved);
        const query = url.searchParams;
        if (!query.has("dev")) {
          query.set("dev", "true");
          importMap.imports["react-dom"] = url.origin + url.pathname + url.query.replace("dev=true", "dev");
        }
      }
    }
  }
  return transform({
    filename,
    code: await Deno.readTextFile("." + filename),
    importMap,
    sourceMap: "inline",
    dev: {
      hmr: { runtime: "/@hmr" },
      refresh: imports?.react && !imports?.preact ? { runtime: "/@refresh" } : undefined,
      prefresh: imports?.preact ? { runtime: "/@prefresh" } : undefined,
    },
  }).code;
}

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [type, ...args] = JSON.parse(line);
    switch (type) {
      case "unocss":
        output(await unocss(...args));
        break;
      case "tsx":
        output(await tsx(...args));
        break;
      default:
        error("Unknown loader type: " + type);
    }
  } catch (e) {
    error(e.message);
  }
}
