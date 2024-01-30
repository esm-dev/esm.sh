import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";
import { allGrammars, allThemes, initShiki } from "./shiki";

const defaultConfig = { theme: "vitesse-dark" };

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

export async function init(options: {
  themes?: string[];
  preloadGrammars?: string[];
  customGrammars?: { name: string }[];
} = {}) {
  let themes = (options.themes || [defaultConfig.theme]).filter((name) =>
    allThemes.some((t) => t.name === name)
  );
  if (themes.length > 0) {
    defaultConfig.theme = themes[0];
  } else {
    themes = [defaultConfig.theme];
  }

  const setupDataMap = Object.fromEntries(
    await Promise.all(
      allGrammars.filter((g) => !!lspIndex[g.name]?.api).map(
        async (g) => {
          const lang = g.name;
          const { init } = await import(
            new URL(`./lsp/${lspIndex[lang].id}/api.js`, import.meta.url).href
          );
          return [lang, init(monaco, lang)];
        },
      ),
    ),
  );

  await initShiki(monaco, {
    ...options,
    themes,
    onLanguage: async (id: string) => {
      if (lspIndex[id]) {
        const { setup } = await import(
          new URL(`./lsp/${lspIndex[id].id}/setup.js`, import.meta.url)
            .href
        );
        setup(id, monaco, setupDataMap[id]);
      }
    },
  });
}

const _create = monaco.editor.create.bind(monaco.editor);
const _createModel = monaco.editor.createModel.bind(monaco.editor);

export function create(
  container: HTMLElement,
  options?: monaco.editor.IStandaloneEditorConstructionOptions,
) {
  return _create(
    container,
    {
      automaticLayout: true,
      minimap: { enabled: false },
      theme: defaultConfig.theme,
      ...options,
    } satisfies typeof options,
  );
}

export function createModel(
  value: string,
  language?: string,
  uri?: string | monaco.Uri,
) {
  return _createModel(
    value,
    language,
    typeof uri === "string" ? monaco.Uri.parse(uri) : uri,
  );
}

// override default create and createModel methods
Object.assign(monaco.editor, { create, createModel });

export * from "monaco-editor-core";
