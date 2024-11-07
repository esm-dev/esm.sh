import { TextLineStream } from "jsr:@std/streams@1.0.7/text-line-stream";

const enc = new TextEncoder();
const output = (type, data) => Deno.stdout.write(enc.encode(">>>" + type + ":" + JSON.stringify(data) + "\n"));

let tsx, unoGenerators;

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
    const ret = await transformVue(filename, sourceCode, importMap, true);
    lang = ret[0];
    sourceCode = ret[1];
  }
  if (!tsx) {
    tsx = import("npm:@esm.sh/tsx@1.0.1").then(async ({ init, transform }) => {
      await init();
      return { transform };
    });
  }
  const { transform } = await tsx;
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

async function transformVue(filename, sourceCode, importMap, isDev) {
  const { transform } = await import("npm:@esm.sh/vue-loader@1.0.3");
  const ret = await transform(filename, sourceCode, {
    imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + getVueVersion(importMap)) },
    isDev,
    devRuntime: isDev ? "/@vdr" : undefined,
  });
  return [ret.lang, ret.code];
}

function getVueVersion(importMap) {
  const vueUrl = importMap?.imports?.vue;
  if (vueUrl && (vueUrl.startsWith("https://") || vueUrl.startsWith("http://"))) {
    const url = new URL(vueUrl);
    const m = url.pathname.match(/^\/\*?vue@([~\^]?[\w\+\-\.]+)(\/|\?|&|$)/);
    if (m) {
      return m[1];
    }
  }
  // default to vue@3
  return "3";
}

async function unocss(config, content) {
  if (!unoGenerators) {
    unoGenerators = new Map();
  }
  const generatorKey = config?.filename ?? "-";
  let uno = unoGenerators.get(generatorKey);
  if (!uno || uno.configCSS !== config?.css) {
    uno = import("npm:@esm.sh/unocss@0.1.0").then(({ init }) => init(config?.css));
    uno.configCSS = config?.css;
    unoGenerators.set(generatorKey, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return await generate();
}

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [type, ...args] = JSON.parse(line);
    switch (type) {
      case "unocss":
        output("css", await unocss(...args));
        break;
      case "module":
        output("js", await transformModule(...args));
        break;
      case "vue":
        output(...(await transformVue(...args)));
        break;
      default:
        output("error", "Unknown loader type: " + type);
    }
  } catch (e) {
    output("error", e.message);
  }
}
