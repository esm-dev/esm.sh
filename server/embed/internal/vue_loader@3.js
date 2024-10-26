import { compileScript, compileStyleAsync, parse, rewriteDefault } from "vue/compiler-sfc";
const { stdin, stdout } = process;

function readStdin() {
  return new Promise((resolve) => {
    let buf = "";
    stdin.setEncoding("utf8");
    stdin.on("data", (chunk) => {
      buf += chunk;
    });
    stdin.on("end", () => resolve(buf));
  });
}

async function computeHash(input) {
  const buffer = new Uint8Array(
    await crypto.subtle.digest("SHA-1", new TextEncoder().encode(input)),
  );
  return [...buffer].map((b) => b.toString(16).padStart(2, "0")).join("");
}

async function load() {
  try {
    const [filename, code] = JSON.parse(await readStdin());
    const stringify = JSON.stringify;
    const specificHash = await computeHash(filename);
    const id = specificHash.slice(0, 10);
    const { descriptor } = parse(code, { filename });
    const scriptLang = (descriptor.script && descriptor.script.lang) || (descriptor.scriptSetup && descriptor.scriptSetup.lang);
    const isTS = scriptLang === "ts";
    if (scriptLang && !isTS) {
      throw new Error(`VueSFCLoader: Only lang="ts" is supported for <script> blocks.`);
    }
    if (descriptor.styles.some((style) => style.module)) {
      throw new Error(`VueSFCLoader: <style module> is not supported yet.`);
    }
    const expressionPlugins = isTS ? ["typescript"] : undefined;
    const templateOptions = {
      id,
      filename: descriptor.filename,
      scoped: descriptor.styles.some((s) => s.scoped),
      slotted: descriptor.slotted,
      isProd: true,
      ssr: false,
      ssrCssVars: descriptor.cssVars,
      compilerOptions: { expressionPlugins },
    };
    const compiledScript = compileScript(descriptor, {
      inlineTemplate: true,
      id,
      templateOptions,
    });
    const mainScript = rewriteDefault(compiledScript.content, "__sfc__", expressionPlugins);
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
      // if (result.map) {
      //   css += "//# sourceMappingURL=data:application/json;charset=utf-8;base64,";
      //   css += btoa(stringify(result.map));
      // }
      return css;
    }));

    const output = [mainScript];
    output.push(`__sfc__.__file = ${stringify(filename)};`);
    if (descriptor.styles.length > 0) {
      if (descriptor.styles.some((s) => s.scoped)) {
        output.push(`__sfc__.__scopeId = "data-v-${id}";`);
      }
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
    output.push("export default __sfc__;");
    stdout.write(JSON.stringify({ code: output.join("\n") }));
  } catch (err) {
    stdout.write(JSON.stringify({ error: err.message, stack: err.stack }));
  }
  process.exit(0);
}

load();
