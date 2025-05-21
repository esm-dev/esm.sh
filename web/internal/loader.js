import { TextLineStream } from "jsr:@std/streams@1.0.9/text-line-stream";

const enc = new TextEncoder();
const output = (type, data) => Deno.stdout.write(enc.encode(">>>" + type + ":" + JSON.stringify(data) + "\n"));
const once = {};

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [type, ...args] = JSON.parse(line);
    switch (type) {
      case "tsx": {
        output("js", await tsx(...args));
        break;
      }
      case "svelte": {
        const [lang, code] = await transformSvelte(...args);
        output(lang, code);
        break;
      }
      case "vue": {
        const [lang, code] = await transformVue(...args);
        output(lang, code);
        break;
      }
      case "unocss": {
        output("css", await unocss(...args));
        break;
      }
      default: {
        output("error", "Unknown loader type: " + type);
      }
    }
  } catch (e) {
    output("error", e.message);
  }
}

// transform TypeScript/JSX/TSX to JavaScript with HMR support
async function tsx(filename, importMap, sourceCode, isDev) {
  const imports = importMap?.imports;
  if (imports && isDev) {
    // add `?dev` query to `react-dom` and `vue` imports for development mode
    for (const [specifier, url] of Object.entries(imports)) {
      if (
        (specifier === "react" || specifier === "react-dom" || specifier === "react-dom/client" || specifier === "vue")
        && (url.startsWith("https://") || url.startsWith("http://"))
      ) {
        const u = new URL(url);
        const q = u.searchParams;
        if (!q.has("dev")) {
          q.set("dev", "true");
          imports[specifier] = u.origin + u.pathname + u.search.replace("dev=true", "dev");
        }
      }
    }
  }
  let lang = filename.endsWith(".md?jsx") ? "jsx" : undefined;
  let code = sourceCode ?? await Deno.readTextFile("." + filename);
  let map = undefined;
  if (filename.endsWith(".svelte") || filename.endsWith(".md?svelte")) {
    [lang, code, map] = await transformSvelte(filename, code, importMap, isDev);
  } else if (filename.endsWith(".vue") || filename.endsWith(".md?vue")) {
    [lang, code, map] = await transformVue(filename, code, importMap, isDev);
  }
  if (!once.tsxWasm) {
    once.tsxWasm = import("npm:@esm.sh/tsx@1.2.0").then(async (m) => {
      await m.init();
      return m;
    });
  }
  const react = imports?.react;
  const preact = imports?.preact;
  const ret = (await once.tsxWasm).transform({
    filename,
    lang,
    code,
    importMap: importMap ?? undefined,
    sourceMap: isDev ? (map ? "external" : "inline") : undefined,
    dev: isDev
      ? {
        hmr: { runtime: "/@hmr" },
        refresh: react && !preact ? { runtime: "/@refresh" } : undefined,
        prefresh: preact && !react ? { runtime: "/@prefresh" } : undefined,
        jsxSource: (react || preact) ? { fileName: Deno.cwd() + filename } : undefined,
      }
      : undefined,
  });
  let js = ret.code;
  if (ret.map) {
    if (map) {
      // todo: merge preprocess source map
    }
    js += "\n//# sourceMappingURL=data:application/json;base64," + btoa(ret.map);
  }
  return js;
}

// transform Vue SFC to JavaScript
async function transformVue(filename, sourceCode, importMap, isDev) {
  const { transform } = await import("npm:@esm.sh/vue-compiler@1.0.1");
  const ret = await transform(filename, sourceCode, {
    imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + getPackageVersion(importMap, "vue", "3")) },
    isDev,
    devRuntime: isDev ? "/@vdr" : undefined,
  });
  return [ret.lang, ret.code, ret.map];
}

// transform Svelte SFC to JavaScript
async function transformSvelte(filename, sourceCode, importMap, isDev) {
  const { compile, VERSION } = await import("npm:svelte@" + getPackageVersion(importMap, "svelte", "5") + "/compiler");
  const majorVersion = parseInt(VERSION.split(".", 1)[0]);
  if (majorVersion < 5) {
    throw new Error("Unsupported Svelte version: " + VERSION + ". Please use svelte@5 or higher.");
  }
  const { js } = compile(sourceCode, {
    filename,
    css: "injected",
    dev: isDev,
    hmr: isDev,
  });
  return ["js", js.code, js.map];
}

// get the package version from the import map
function getPackageVersion(importMap, pkgName, defaultVersion) {
  const url = importMap?.imports?.[pkgName];
  if (url && (url.startsWith("https://") || url.startsWith("http://"))) {
    const { pathname } = new URL(url);
    const m = pathname.match(/^\/\*?(svelte|vue)@([~\^]?[\w\+\-\.]+)(\/|\?|&|$)/);
    if (m) {
      return m[2];
    }
  }
  return defaultVersion;
}

// generate unocss for the given content
async function unocss(_id, content, config) {
  const generatorId = config?.filename ?? ".";
  if (!once.unoGenerators) {
    once.unoGenerators = new Map();
  }
  let uno = once.unoGenerators.get(generatorId);
  if (!uno || uno.configCSS !== config?.css) {
    uno = import("npm:@esm.sh/unocss@0.5.0").then(({ init }) => init({ configCSS: config?.css }));
    uno.configCSS = config?.css;
    once.unoGenerators.set(generatorId, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return await generate();
}
