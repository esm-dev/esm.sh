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
async function tsx(filename, importMap, sourceCode, isDev) {
  const imports = importMap?.imports;
  const devImports = {};
  if (imports && isDev) {
    // add `dev` query to `react`, `react-dom` and `vue` imports for development mode
    for (const [specifier, url] of Object.entries(imports)) {
      const isReact = specifier === "react" || specifier.startsWith("react/");
      const isReactDOM = specifier === "react-dom" || specifier.startsWith("react-dom/");
      const isVue = specifier === "vue";
      if (isHttpUrl(url) && (isReact || isReactDOM || isVue)) {
        const [pkgName] = specifier.split("/", 1);
        const moduleUrl = new URL(url);
        const { pathname } = moduleUrl;
        const firstSegment = pathname.split("/", 2)[1];
        if (firstSegment === pkgName || firstSegment.startsWith(pkgName + "@")) {
          const version = firstSegment.split("@")[1];
          // replace extension `.mjs`  with `.development.mjs`
          // or add `dev` query to the module url
          if (pathname.endsWith(".mjs") && version) {
            moduleUrl.pathname = pathname.slice(0, -4) + ".development.mjs";
            devImports[specifier] = moduleUrl.toString();
          } else {
            moduleUrl.searchParams.set("dev", "TRUE");
            devImports[specifier] = moduleUrl.toString().replace("dev=TRUE", "dev");
          }
        }
      }
    }
  }
  let jsxImportSource = undefined;
  if (imports) {
    let jsxImportSourceUrl = undefined;
    for (const pkg of ["react", "preact", "solid-js", "mono-jsx/dom", "mono-jsx", "vue"]) {
      jsxImportSourceUrl = imports[pkg + "/jsx-runtime"] || imports[pkg + "/"];
      if (jsxImportSourceUrl) {
        jsxImportSource = pkg;
        break;
      }
    }
    // ensure `jsx-dev-runtime` is included in the import map
    if (isDev && jsxImportSourceUrl && !imports[jsxImportSource + "/jsx-dev-runtime"]) {
      const version = getPackageVersionFromUrl(jsxImportSourceUrl);
      if (version && jsxImportSourceUrl.endsWith("/jsx-runtime.mjs")) {
        devImports[jsxImportSource + "/jsx-dev-runtime"] = jsxImportSourceUrl.slice(0, -16) + "/jsx-dev-runtime.development.mjs";
      } else if (version) {
        const { origin } = new Url(jsxImportSourceUrl);
        devImports[jsxImportSource + "/jsx-dev-runtime"] = origin + "/" + jsxImportSource + "@" + version + "/jsx-dev-runtime";
      }
    }
  }
  output("debug", JSON.stringify(devImports));
  let lang = filename.endsWith(".md?jsx") ? "jsx" : undefined;
  let code = sourceCode ?? await Deno.readTextFile("." + filename);
  let map = undefined;
  if (filename.endsWith(".svelte") || filename.endsWith(".md?svelte")) {
    [lang, code, map] = await transformSvelte(filename, code, importMap, isDev);
  } else if (filename.endsWith(".vue") || filename.endsWith(".md?vue")) {
    [lang, code, map] = await transformVue(filename, code, importMap, isDev);
  }
  if (!once.tsxWasm) {
    once.tsxWasm = import("npm:@esm.sh/tsx@1.5.2").then(async (m) => {
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
    imports: { "@vue/compiler-sfc": import("npm:@vue/compiler-sfc@" + getPackageVersionFromUrl(importMap?.imports?.vue, "3")) },
    isDev,
    devRuntime: isDev ? "/@vdr" : undefined,
  });
  return [ret.lang, ret.code, ret.map];
}

// transform Svelte SFC to JavaScript
async function transformSvelte(filename, sourceCode, importMap, isDev) {
  const { compile, VERSION } = await import("npm:svelte@" + getPackageVersionFromUrl(importMap?.imports?.svelte, "5") + "/compiler");
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
function getPackageVersionFromUrl(url, defaultVersion) {
  if (isHttpUrl(url)) {
    const { pathname } = new URL(url);
    const m = pathname.match(/^\/\*?[a-z\-]+@([~\^]?[\w\+\-\.]+)(\/|\?|&|$)/);
    if (m) {
      return m[1];
    }
  }
  return defaultVersion;
}

// check if the url is a http url
function isHttpUrl(url) {
  return typeof url === "string" && url.startsWith("https://") || url.startsWith("http://");
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
    uno = import("npm:@esm.sh/unocss@0.5.4").then(({ init }) => init({ configCSS: config?.css }));
    uno.configCSS = config?.css;
    once.unoGenerators.set(generatorId, uno);
  }
  const { update, generate } = await uno;
  await update(content);
  return generate();
}
