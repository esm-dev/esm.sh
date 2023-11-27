/** @version: 3.3.9 */

import {
  type CompilerOptions,
  compileScript,
  compileStyleAsync,
  parse,
  rewriteDefault,
  type SFCAsyncStyleCompileOptions,
  type SFCScriptCompileOptions,
  type SFCTemplateCompileOptions,
} from "https://esm.sh/v135/@vue/compiler-sfc@3.3.9";

interface Options {
  script?: Omit<SFCScriptCompileOptions, "id">;
  template?: Partial<SFCTemplateCompileOptions>;
  style?: Partial<SFCAsyncStyleCompileOptions>;
  isDev?: boolean;
  importMap?: { imports?: Record<string, string> };
  sourceMap?: boolean;
}

const transform = async (
  specifier: string,
  content: string,
  options?: Options,
) => {
  const specificHash = await computeHash(new TextEncoder().encode(specifier));
  const id = specificHash.slice(0, 8);
  const { descriptor } = parse(content, {
    filename: specifier,
    sourceMap: options?.sourceMap,
  });
  const scriptLang = (descriptor.script && descriptor.script.lang) ||
    (descriptor.scriptSetup && descriptor.scriptSetup.lang);
  const isTS = scriptLang === "ts";
  if (scriptLang && !isTS) {
    throw new Error(
      `VueSFCLoader: Only lang="ts" is supported for <script> blocks.`,
    );
  }
  if (descriptor.styles.some((style) => style.module)) {
    console.warn(`VueSFCLoader: <style module> is not supported yet.`);
  }
  const expressionPlugins: CompilerOptions["expressionPlugins"] = isTS
    ? ["typescript"]
    : undefined;
  const templateOptions: Omit<SFCTemplateCompileOptions, "source"> = {
    ...options?.template,
    id,
    filename: descriptor.filename,
    scoped: descriptor.styles.some((s) => s.scoped),
    slotted: descriptor.slotted,
    isProd: !options?.isDev,
    ssr: false,
    ssrCssVars: descriptor.cssVars,
    compilerOptions: {
      ...options?.template?.compilerOptions,
      runtimeModuleName:
        options?.template?.compilerOptions?.runtimeModuleName ??
          options?.importMap?.imports?.["vue"],
      expressionPlugins,
    },
  };
  const compiledScript = compileScript(descriptor, {
    inlineTemplate: true,
    ...options?.script,
    id,
    templateOptions,
  });
  const mainScript = rewriteDefault(
    compiledScript.content,
    "__sfc__",
    expressionPlugins,
  );
  const output = [mainScript];
  output.push(`__sfc__.__file = ${JSON.stringify(specifier)};`);
  if (descriptor.styles.some((s) => s.scoped)) {
    output.push(`__sfc__.__scopeId = ${JSON.stringify(`data-v-${id}`)};`);
  }

  const styles = await Promise.all(descriptor.styles.map(async (style) => {
    const result = await compileStyleAsync({
      ...options?.style,
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
      css +=
        "\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
      css += btoa(JSON.stringify(result.map));
    }
    return css;
  }));
  if (styles.length) {
    output.push("const __mounted__ = __sfc__.mounted;");
    output.push("__sfc__.mounted = function() { ");
    output.push("const rootEl = this.$root.$el;");
    output.push(
      "const doc = rootEl.getRootNode ? rootEl.getRootNode() : rootEl.ownerDocument;",
    );
    styles.forEach((css) => {
      output.push("const style = document.createElement('style');");
      output.push(`style.textContent = ${JSON.stringify(css)};`);
      output.push(`style.setAttribute('data-v-${id}', '');`);
      output.push("(doc.head || doc).appendChild(style);");
    });
    output.push("__mounted__ && __mounted__.call(this);");
    output.push("};");
  }
  output.push(`export default __sfc__;`);

  return {
    code: output.join("\n"),
    map: compiledScript.map?.toString(),
  };
};

async function computeHash(input: Uint8Array): Promise<string> {
  const buffer = new Uint8Array(
    await crypto.subtle.digest(
      "SHA-1",
      input,
    ),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

export default {
  name: "vue",
  setup(hot: any) {
    hot.onLoad(
      /\.vue$/,
      (url: URL, source: string, options: Record<string, any> = {}) => {
        const { isDev, importMap } = options;
        return transform(url.pathname, source, {
          isDev,
          importMap,
          sourceMap: !!isDev,
        });
      },
    );
  },
};
