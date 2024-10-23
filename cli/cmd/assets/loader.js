import { TextLineStream } from "jsr:@std/streams@1.0.7/text-line-stream";

const enc = new TextEncoder();
const output = (data) => Deno.stdout.write(enc.encode(">>>" + JSON.stringify(data) + "\n"));
const error = (message) => Deno.stdout.write(enc.encode(">>!" + JSON.stringify(message) + "\n"));

let uno;
async function unocss(configCSS, data) {
  if (!uno || uno.configCSS !== configCSS) {
    const { init } = await import("npm:esm-unocss@0.6.0");
    uno = await init(configCSS);
    uno.configCSS = configCSS;
  }
  if (await uno.update(data)) {
    const ret = await uno.generate();
    return ret.css;
  }
  return "";
}

let esmTsx;
async function tsx(filename, importMap) {
  if (!esmTsx) {
    esmTsx = import("npm:esm-tsx@1.2.3").then(async ({ init, transform }) => {
      await init();
      return { transform };
    });
  }
  const { transform } = await esmTsx;
  const imports = importMap?.imports;
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
        error(`Unknown loader type: ${type}`);
    }
  } catch (e) {
    error(e.message);
  }
}
