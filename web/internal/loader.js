import { TextLineStream } from "jsr:@std/streams@1.0.9/text-line-stream";

const once = {};
const enc = new TextEncoder();
const dec = new TextDecoder();
const output = (type, data) => Deno.stdout.write(enc.encode(">>>" + type + ":" + JSON.stringify(data) + "\n"));
const error = (message) => output("error", message);

for await (const line of Deno.stdin.readable.pipeThrough(new TextDecoderStream()).pipeThrough(new TextLineStream())) {
  try {
    const [loader, ...args] = JSON.parse(line);
    switch (loader) {
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
      case "tailwind": {
        output("css", await tailwindCSS(...args));
        break;
      }
      case "unocss": {
        output("css", await unocss(...args));
        break;
      }
      default: {
        error("Unknown loader: " + loader);
      }
    }
  } catch (e) {
    error(e.message);
  }
}

// transform TypeScript/JSX/TSX to JavaScript with HMR support
async function tsx(filename, sourceCode, options) {
  const { isDev, react, preact, jsxImportSource } = options ?? {};
  let lang = filename.endsWith(".md?jsx") ? "jsx" : undefined;
  let code = sourceCode ?? await Deno.readTextFile("." + filename);
  let map = options?.map;
  if (filename.endsWith(".svelte") || filename.endsWith(".md?svelte")) {
    [lang, code, map] = await transformSvelte(filename, code, options);
  } else if (filename.endsWith(".vue") || filename.endsWith(".md?vue")) {
    [lang, code, map] = await transformVue(filename, code, options);
  }
  if (!once.tsxWasm) {
    once.tsxWasm = import("npm:@esm.sh/tsx@1.5.3").then(async (m) => {
      await m.init();
      return m;
    });
  }

  const ret = (await once.tsxWasm).transform({
    filename,
    lang,
    code,
    jsxImportSource,
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
  let js = dec.decode(ret.code);
  if (map && ret.map) {
    // todo: merge source maps
    js += "\n//# sourceMappingURL=data:application/json;base64," + btoa(dec.decode(ret.map));
  }
  return js;
}

// transform Vue SFC to JavaScript
async function transformVue(filename, sourceCode, options) {
  const { isDev, vueVersion } = options ?? {};
  if (!vueVersion) {
    throw new Error("`vueVersion` option is required");
  }
  const { transform } = await import("npm:@esm.sh/vue-compiler@1.0.1");
  const code = sourceCode ?? await Deno.readTextFile("." + filename);
  const ret = await transform(filename, code, {
    imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + vueVersion) },
    isDev,
    devRuntime: isDev ? "/@vdr" : undefined,
  });
  return [ret.lang, ret.code, ret.map];
}

// transform Svelte SFC to JavaScript
async function transformSvelte(filename, sourceCode, options) {
  const { isDev, svelteVersion } = options ?? {};
  if (!svelteVersion) {
    throw new Error("`svelteVersion` option is required");
  }
  const { compile, VERSION } = await import("npm:svelte@" + svelteVersion + "/compiler");
  const code = sourceCode ?? await Deno.readTextFile("." + filename);
  const majorVersion = parseInt(VERSION.split(".", 1)[0]);
  if (majorVersion < 5) {
    throw new Error("Unsupported svelte version: " + VERSION + ". Please use svelte@5 or higher.");
  }
  const { js } = compile(code, {
    filename,
    css: "injected",
    dev: isDev,
    hmr: isDev,
  });
  return ["js", js.code, js.map];
}

// generate css for the given content using tailwindcss
async function tailwindCSS(_id, content, config) {
  const compilerId = config?.filename ?? ".";
  if (!once.tailwindCompilers) {
    once.tailwindCompilers = new Map();
  }
  if (!once.tailwind) {
    once.tailwind = import("npm:tailwindcss@4.1.18");
  }
  if (!once.oxide) {
    once.oxide = import("npm:@esm.sh/oxide-wasm@0.1.4").then(({ init, extract }) => init().then(() => ({ extract })));
  }
  let compiler = once.tailwindCompilers.get(compilerId);
  if (!compiler || compiler.configCSS !== config?.css) {
    compiler = (async () => {
      const { compile } = await once.tailwind;
      return compile(config.css, {
        async loadStylesheet(id, sheetBase) {
          switch (id) {
            case "tailwindcss": {
              if (!once.tailwindIndexCSS) {
                once.tailwindIndexCSS = fetch("https://esm.sh/tailwindcss@4.1.18/index.css").then(res => res.text());
              }
              const css = await once.tailwindIndexCSS;
              return {
                content: css,
              };
            }
            case "tw-animate-css": {
              if (!once.twAnimateCSS) {
                once.twAnimateCSS = fetch("https://esm.sh/tw-animate-css@1.4.0/dist/tw-animate.css").then(res => res.text());
              }
              const css = await once.twAnimateCSS;
              return {
                content: css,
              };
            }
          }
          // todo: load and cache other css from npm
          throw new Error("could not find stylesheet id: " + id + ", sheetBase: " + sheetBase);
          return null;
        },
      });
    })();
    compiler.configCSS = config?.css;
    once.tailwindCompilers.set(compilerId, compiler);
  }
  const { extract } = await once.oxide;
  return (await compiler).build(extract(content));
}

// generate css for the given content using unocss
async function unocss(_id, content, config) {
  const generatorId = config?.filename ?? ".";
  if (!once.unoGenerators) {
    once.unoGenerators = new Map();
  }
  let uno = once.unoGenerators.get(generatorId);
  if (!uno || uno.configCSS !== config?.css) {
    uno = import("npm:@esm.sh/unocss@0.6.0").then(({ init }) => init({ configCSS: config?.css }));
    uno.configCSS = config?.css;
    once.unoGenerators.set(generatorId, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return generate();
}
