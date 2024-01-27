import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";

const shikiVersion = "1.0.0-beta.0";
const defaultConfig = {
  theme: "vitesse-dark",
  languages: ["html", "css", "json", "javascript", "typescript", "markdown"],
};

// @ts-ignore - MonacoEnvironment
globalThis.MonacoEnvironment = {
  getWorker: (_, label: string) => {
    let filename = "./editor-worker.js";
    if (lspIndex[label]) {
      filename = `./lsp/${lspIndex[label]}/worker.js`;
    }
    return new Worker(
      new URL(filename, import.meta.url),
      { type: "module" },
    );
  },
};

export async function init(
  options: {
    themes?: string[];
    languages?: string[];
  } = {},
) {
  if (options.themes?.length > 0) {
    defaultConfig.theme = options.themes[0];
  }
  const [
    { getHighlighter },
    { shikiToMonaco },
  ] = await Promise.all([
    import(`https://esm.sh/shiki@${shikiVersion}`),
    import(`https://esm.sh/@shikijs/monaco@${shikiVersion}`),
  ]);
  const themes = options.themes ?? [defaultConfig.theme];
  const langs = options.languages ?? defaultConfig.languages;
  const highlighter = await getHighlighter({ langs, themes });
  if (langs) {
    for (const id of langs) {
      monaco.languages.register({ id });
      monaco.languages.onLanguage(id, async () => {
        if (lspIndex[id]) {
          const { setup } = await import(
            new URL(`./lsp/${lspIndex[id]}/setup.js`, import.meta.url).href
          );
          setup(id, monaco);
        }
      });
    }
  }
  shikiToMonaco(highlighter, monaco);
}

const createEditor = monaco.editor.create;
monaco.editor.create = function (container, options) {
  return createEditor(container, { theme: defaultConfig.theme, ...options });
};

export * from "monaco-editor-core";
