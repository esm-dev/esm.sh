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
        output("css", await tailwind(...args));
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
async function tsx(filename, importMap, sourceCode, isDev) {
  const imports = importMap?.imports;
  const devImports = {};
  if (imports && isDev) {
    // add `?dev` query to `react-dom` and `vue` imports for development mode
    for (const [specifier, url] of Object.entries(imports)) {
      const isReact = specifier === "react" || specifier === "react/" || specifier.startsWith("react/");
      if (
        (
          isReact
          || specifier === "react-dom" || specifier === "react-dom/" || specifier.startsWith("react-dom/")
          || specifier === "vue"
        ) && (url.startsWith("https://") || url.startsWith("http://"))
      ) {
        const [pkgName, subModule] = specifier.split("/");
        const { pathname } = new URL(url);
        const seg1 = pathname.split("/")[1];
        if (seg1 === pkgName || seg1.startsWith(pkgName + "@")) {
          const version = seg1.split("@")[1];
          if (specifier.endsWith("/") || !version) {
            devImports[specifier] = "https://esm.sh/" + pkgName + (version ? "@" + version : "@latest") + "&dev"
              + (subModule ? "/" + subModule : "") + "/";
          } else {
            devImports[specifier] = "https://esm.sh/" + pkgName + "@" + version + "/es2022/" + (subModule || pkgName)
              + ".development.mjs";
          }
          if (isReact && version) {
            devImports["react/jsx-dev-runtime"] = "https://esm.sh/react@" + version + "/es2022/jsx-dev-runtime.development.mjs";
          }
        }
      }
    }
  }
  let jsxImportSource = undefined;
  if (["react/", "react/jsx-runtime", "react/jsx-dev-runtime"].some(s => !!(imports?.[s]))) {
    jsxImportSource = "react";
  } else if (["preact/", "preact/jsx-runtime", "preact/jsx-dev-runtime"].some(s => !!(imports?.[s]))) {
    jsxImportSource = "preact";
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
    once.tsxWasm = import("npm:@esm.sh/tsx@1.5.1").then(async (m) => {
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
    jsxImportSource,
    importMap: isDev ? { imports: devImports } : undefined,
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
  if (ret.map) {
    if (map) {
      // todo: merge preprocess source map
    }
    js += "\n//# sourceMappingURL=data:application/json;base64," + btoa(dec.decode(ret.map));
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

async function tailwind(_id, content, config) {
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

// generate unocss for the given content
async function unocss(_id, content, config) {
  const generatorId = config?.filename ?? ".";
  if (!once.unoGenerators) {
    once.unoGenerators = new Map();
  }
  let uno = once.unoGenerators.get(generatorId);
  if (!uno || uno.configCSS !== config?.css) {
    uno = import("npm:@esm.sh/unocss@0.5.4").then(({ init }) => init({ configCSS: config?.css }));
    uno.configCSS = config?.css;
    once.unoGenerators.set(generatorId, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return generate();
}
