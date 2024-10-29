import { TextLineStream } from "jsr:@std/streams@1.0.7/text-line-stream";

const enc = new TextEncoder();
const output = (data, type = ">") => Deno.stdout.write(enc.encode(">>" + type + JSON.stringify(data) + "\n"));
const error = (message) => output(message, "!");

let esmTsx;
async function transformModule(filename, importMap) {
  const imports = importMap?.imports;
  if (imports) {
    for (const [specifier, resolved] of Object.entries(imports)) {
      if (
        (specifier === "react-dom" || specifier === "react-dom/client" || specifier === "vue")
        && (resolved.startsWith("https://") || resolved.startsWith("http://"))
      ) {
        const url = new URL(resolved);
        const query = url.searchParams;
        if (!query.has("dev")) {
          query.set("dev", "true");
          imports[specifier] = url.origin + url.pathname + url.search.replace("dev=true", "dev");
        }
      }
    }
  }
  let sourceCode = await Deno.readTextFile("." + filename);
  let lang = undefined; // by default use file extension to determine lang
  if (filename.endsWith(".vue")) {
    const { transform } = await import("npm:esm-vue-sfc-compiler@0.4.7");
    const vueUrl = imports?.vue;
    let vueVersion = undefined;
    if (vueUrl && (vueUrl.startsWith("https://") || vueUrl.startsWith("http://"))) {
      const url = new URL(vueUrl);
      const m = url.pathname.match(/^\/\*?vue@([~\^]?[\w\+\-\.]+)(\/|\?|&|$)/);
      if (m) {
        vueVersion = m[1];
      }
    }
    if (!vueVersion) {
      throw new Error("'vue' not specified in import map or invalid version");
    }
    const ret = await transform(filename, sourceCode, {
      imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + vueVersion) },
      devRuntime: "/@vdr",
      isDev: true,
    });
    sourceCode = ret.code;
    lang = ret.lang;
  }
  if (!esmTsx) {
    esmTsx = import("npm:esm-tsx@1.3.1").then(async ({ init, transform }) => {
      await init();
      return { transform };
    });
  }
  const { transform } = await esmTsx;
  return transform({
    filename,
    lang,
    code: sourceCode,
    importMap,
    sourceMap: "inline",
    dev: {
      hmr: { runtime: "/@hmr" },
      refresh: imports?.react && !imports?.preact ? { runtime: "/@refresh" } : undefined,
      prefresh: imports?.preact ? { runtime: "/@prefresh" } : undefined,
    },
  }).code;
}

let uno;
async function unocss(configCSS, code) {
  if (!uno || uno.configCSS !== configCSS) {
    uno = import("npm:esm-unocss@0.8.0").then(({ init }) => init(configCSS));
    uno.configCSS = configCSS;
  }
  const { update, generate } = await uno;
  await update(code);
  return await generate();
}

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [type, ...args] = JSON.parse(line);
    switch (type) {
      case "unocss":
        output(await unocss(...args));
        break;
      case "module":
        output(await transformModule(...args));
        break;
      default:
        error("Unknown loader type: " + type);
    }
  } catch (e) {
    error(e.message);
  }
}
