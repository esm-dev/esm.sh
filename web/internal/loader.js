import { TextLineStream } from "jsr:@std/streams@1.0.9/text-line-stream";

const enc = new TextEncoder();
const regexpModulePath = /^\/\*?(svelte|vue)@([~\^]?[\w\+\-\.]+)(\/|\?|&|$)/;
const output = (type, data) => Deno.stdout.write(enc.encode(">>>" + type + ":" + JSON.stringify(data) + "\n"));

let tsx;
let unoGenerators;

async function transformModule(filename, importMap, sourceCode, isDev) {
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
  if (!tsx) {
    tsx = import("npm:@esm.sh/tsx@1.2.0").then(async ({ init, transform }) => {
      await init();
      return { transform };
    });
  }
  const { transform } = await tsx;
  const react = imports?.react;
  const preact = imports?.preact;
  const ret = transform({
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

async function transformSvelte(filename, sourceCode, importMap, isDev) {
  const { compile, VERSION } = await import(`npm:svelte@${getSvelteVersion(importMap)}/compiler`);
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

async function transformVue(filename, sourceCode, importMap, isDev) {
  const { transform } = await import("npm:@esm.sh/vue-compiler@1.0.1");
  const ret = await transform(filename, sourceCode, {
    imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + getVueVersion(importMap)) },
    isDev,
    devRuntime: isDev ? "/@vdr" : undefined,
  });
  return [ret.lang, ret.code, ret.map];
}

function getSvelteVersion(importMap) {
  const svelteUrl = importMap?.imports?.svelte;
  if (svelteUrl && isHttpSpecifier(svelteUrl)) {
    const url = new URL(svelteUrl);
    const m = url.pathname.match(regexpModulePath);
    if (m) {
      return m[2];
    }
  }
  // default to svelte@5
  return "5";
}

function getVueVersion(importMap) {
  const vueUrl = importMap?.imports?.vue;
  if (vueUrl && isHttpSpecifier(vueUrl)) {
    const url = new URL(vueUrl);
    const m = url.pathname.match(regexpModulePath);
    if (m) {
      return m[2];
    }
  }
  // default to vue@3
  return "3";
}

function isHttpSpecifier(specifier) {
  return typeof specifier === "string" && specifier.startsWith("https://") || specifier.startsWith("http://");
}

async function unocss(_id, content, config) {
  const generatorId = config?.filename ?? ".";
  if (!unoGenerators) {
    unoGenerators = new Map();
  }
  let uno = unoGenerators.get(generatorId);
  if (!uno || uno.configCSS !== config?.css) {
    uno = import("npm:@esm.sh/unocss@0.5.0-beta.3").then(({ init }) => init({ configCSS: config?.css }));
    uno.configCSS = config?.css;
    unoGenerators.set(generatorId, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return await generate();
}

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [type, ...args] = JSON.parse(line);
    switch (type) {
      case "module":
        output("js", await transformModule(...args));
        break;
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
