import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";

const shikiVersion = "1.0.0-beta.0";
const defaultConfig = {
  theme: "vitesse-dark",
  languages: ["html", "css", "json", "javascript", "typescript", "markdown"],
};

// @ts-expect-error global `MonacoEnvironment` variable not declared
globalThis.MonacoEnvironment = {
  getWorker: (_, label: string) => {
    let filename = "./editor-worker.js";
    if (lspIndex[label]) {
      filename = `./lsp/${lspIndex[label].id}/worker.js`;
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
  if (options.themes?.length) {
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
    const setupDatas = Object.fromEntries(
      await Promise.all(langs.map(async (id) => {
        if (lspIndex[id]?.api) {
          const { init } = await import(
            new URL(`./lsp/${lspIndex[id].id}/api.js`, import.meta.url).href
          );
          return [id, init(monaco, id)];
        }
        return [id, null];
      })),
    );
    for (const id of langs) {
      monaco.languages.register({ id });
      monaco.languages.onLanguage(id, async () => {
        if (lspIndex[id]) {
          const { setup } = await import(
            new URL(`./lsp/${lspIndex[id].id}/setup.js`, import.meta.url)
              .href
          );
          setup(id, monaco, setupDatas[id]);
        }
      });
    }
  }
  shikiToMonaco(highlighter, monaco);
}

const createEditor = monaco.editor.create;
monaco.editor.create = function (container, options) {
  return createEditor(container, {
    minimap: { enabled: false },
    theme: defaultConfig.theme,
    ...options,
  });
};

export * from "monaco-editor-core";
