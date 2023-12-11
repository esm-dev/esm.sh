/** @version: 3.3.9 */

import type { Hot, ImportMap } from "../types/hot.d.ts";
import {
  type CompilerOptions,
  compileScript,
  compileStyleAsync,
  parse,
  rewriteDefault,
  type SFCTemplateCompileOptions,
} from "https://esm.sh/@vue/compiler-sfc@3.3.9";

interface Options {
  isDev: boolean;
  importMap: ImportMap;
  hmr?: { runtime: string };
}

const compileSFC = async (
  filename: string,
  content: string,
  options: Options,
) => {
  const { importMap, isDev, hmr } = options;
  const stringify = JSON.stringify;
  const specificHash = await computeHash(filename);
  const id = specificHash.slice(0, 10);
  const { descriptor } = parse(content, { filename, sourceMap: isDev });
  const scriptLang = (descriptor.script && descriptor.script.lang) ||
    (descriptor.scriptSetup && descriptor.scriptSetup.lang);
  const isTS = scriptLang === "ts";
  if (scriptLang && !isTS) {
    throw new Error(
      `VueSFCLoader: Only lang="ts" is supported for <script> blocks.`,
    );
  }
  if (descriptor.styles.some((style) => style.module)) {
    throw new Error(`VueSFCLoader: <style module> is not supported yet.`);
  }
  const expressionPlugins: CompilerOptions["expressionPlugins"] = isTS
    ? ["typescript"]
    : undefined;
  const runtimeModuleName = importMap?.imports?.["vue"]
    ? importMap.$support ? "vue" : importMap.imports["vue"]
    : "https://esm.sh/vue@3.3.9";
  const templateOptions: Omit<SFCTemplateCompileOptions, "source"> = {
    id,
    filename: descriptor.filename,
    scoped: descriptor.styles.some((s) => s.scoped),
    slotted: descriptor.slotted,
    isProd: !isDev,
    ssr: false,
    ssrCssVars: descriptor.cssVars,
    compilerOptions: { runtimeModuleName, expressionPlugins },
  };
  const compiledScript = compileScript(descriptor, {
    inlineTemplate: true,
    id,
    templateOptions,
  });
  const mainScript = rewriteDefault(
    compiledScript.content,
    "__sfc__",
    expressionPlugins,
  );

  const output = [mainScript];
  output.push(`__sfc__.__file = ${stringify(filename)};`);
  if (descriptor.styles.some((s) => s.scoped)) {
    output.push(`__sfc__.__scopeId = "data-v-${id}";`);
  }

  if (isDev && hmr) {
    output.push(`import __HOT__ from ${stringify(hmr.runtime)};`);
    output.push(`import.meta.hot = __HOT__(${stringify(filename)});`);
    const mainScriptHash = (await computeHash(mainScript)).slice(0, 10);
    output.push(`__sfc__.__scriptHash = "${mainScriptHash}";`);
    output.push(`__sfc__.__hmrId = "${id}";`);
    output.push(
      `window.__VUE_HMR_RUNTIME__?.createRecord(__sfc__.__hmrId, __sfc__);`,
    );
    output.push(`let __currentScriptHash = "${mainScriptHash}";`);
    output.push(
      `import.meta.hot.accept(({ default: sfc }) => {`,
      `  const rerender = __currentScriptHash === sfc.__scriptHash;`,
      `  __currentScriptHash = sfc.__scriptHash; // update '__currentScriptHash';`,
      `  if (rerender) {`,
      `    __VUE_HMR_RUNTIME__.rerender(sfc.__hmrId, sfc.render);`,
      `  } else {`,
      `    __VUE_HMR_RUNTIME__.reload(sfc.__hmrId, sfc);`,
      `  }`,
      `  const styleEls = __VUE_HMR_RUNTIME__.mountedStyles?.get(sfc.__hmrId);`,
      `  if (styleEls) {`,
      `    const docs = new Set(styleEls.map((el) => { const doc = el.getRootNode ? el.getRootNode() : el.ownerDocument; el.parentNode.removeChild(el); return doc; }));`,
      `    styleEls.length = 0; // flush`,
      `    sfc.styles.forEach((css, idx) => docs.forEach(doc => __addCss__(doc, "vue-css-" + sfc.__hmrId + "-" + idx, css)));`,
      `  }`,
      `});`,
    );
  }

  const styles = await Promise.all(descriptor.styles.map(async (style) => {
    const result = await compileStyleAsync({
      id,
      filename: descriptor.filename,
      source: style.content,
      scoped: style.scoped,
      modules: style.module != null,
      inMap: compiledScript.map,
      isAsync: false,
    });
    if (result.errors.length) {
      // postcss uses pathToFileURL which isn't polyfilled in the browser
      // ignore these errors for now
      const msg = result.errors[0].message;
      if (!msg.includes("pathToFileURL")) {
        console.warn(`VueSFCLoader: ${msg}`);
      }
      // proceed even if css compile errors
      return "";
    }
    let css = result.code;
    if (result.map) {
      css += "//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
      css += btoa(stringify(result.map));
    }
    return css;
  }));
  if (styles.length) {
    output.push("");
    output.push("/* styles */");
    output.push(`__sfc__.styles = ${stringify(styles)};`);
    output.push([
      "const __addCss__ = (doc, id, css) => {",
      "if (doc.getElementById(id)) return;",
      "const style = document.createElement('style');",
      "style.id = id;",
      "style.textContent = css;",
      "(doc.head || doc).appendChild(style);",
      ...(isDev && hmr
        ? [
          "if (window.__VUE_HMR_RUNTIME__) {",
          "const map = __VUE_HMR_RUNTIME__.mountedStyles ?? (__VUE_HMR_RUNTIME__.mountedStyles = new Map());",
          `!map.has("${id}") && map.set("${id}", []);`,
          `map.get("${id}").push(style);`,
          "}",
        ]
        : []),
      "};",
    ].join(""));
    output.push("const __mounted__ = __sfc__.mounted;");
    output.push([
      "__sfc__.mounted = function() {",
      "const rootEl = this.$root.$el;",
      "const doc = rootEl.getRootNode ? rootEl.getRootNode() : rootEl.ownerDocument;",
      `__sfc__.styles.forEach((css, idx) => __addCss__(doc, "vue-css-" + "${id}" + "-" + idx, css));`,
      "__mounted__ && __mounted__.call(this);",
      "};",
    ].join(""));
  }
  output.push(`export default __sfc__;`);

  return {
    code: output.join("\n"),
    map: compiledScript.map?.toString(),
  };
};

async function computeHash(input: string): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      new TextEncoder().encode(input),
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export default {
  name: "vue",
  setup(hot: Hot) {
    // add `?dev` to vue import in dev mode
    if (hot.isDev) {
      hot.onFetch((url: URL, req: Request) => {
        if (
          url.hostname === "esm.sh" &&
          !url.searchParams.has("dev") &&
          req.method === "GET"
        ) {
          const p = url.pathname.split("/");
          const [name] = p[1].split("@");
          return p.length <= 3 && name === "vue";
        }
        return false;
      }, (req: Request) => {
        const url = new URL(req.url);
        url.searchParams.set("dev", "");
        return Response.redirect(url.href.replace("dev=", "dev"), 302);
      });
    }

    // vue sfc loader
    hot.onLoad(
      /\.vue$/,
      (url: URL, source: string, options: Record<string, any> = {}) => {
        const { importMap, isDev } = options;
        const hmrRuntime = importMap.imports?.["@hmrRuntime"];
        return compileSFC(url.pathname, source, {
          isDev,
          importMap,
          hmr: hmrRuntime && {
            runtime: hmrRuntime,
          },
        });
      },
    );
  },
};
