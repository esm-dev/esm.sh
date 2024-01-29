import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";

const shikiVersion = "1.0.0-beta.0";
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
  preloadLanguages?: string[];
  customLanguages?: { name: string }[];
} = {}) {
  if (options.themes?.length) {
    defaultConfig.theme = options.themes[0];
  }
  const [
    { getHighlighterCore },
    { bundledLanguages },
    { bundledThemes },
    { default: loadWasm },
    { shikiToMonaco },
  ] = await Promise.all([
    import(`https://esm.sh/@shikijs/core@${shikiVersion}`),
    import(`https://esm.sh/shiki@${shikiVersion}/langs`),
    import(`https://esm.sh/shiki@${shikiVersion}/themes`),
    import(`https://esm.sh/shiki@${shikiVersion}/wasm?bundle`),
    import(`https://esm.sh/@shikijs/monaco@${shikiVersion}`),
  ]);
  const bundledLanguageIds = Object.keys(bundledLanguages);
  const loadedLanguages = new Set<string>();
  const langs = [];
  const themes = [];
  if (options.customLanguages) {
    for (const lang of options.customLanguages) {
      if (
        typeof lang === "object" && lang !== null && lang.name &&
        !bundledLanguageIds.includes(lang.name)
      ) {
        bundledLanguageIds.push(lang.name);
        loadedLanguages.add(lang.name);
        langs.push(lang);
      }
    }
  }
  for (const theme of options.themes ?? [defaultConfig.theme]) {
    if (typeof theme === "string") {
      if (bundledThemes[theme]) {
        themes.push(bundledThemes[theme]());
      }
    } else if (typeof theme === "object" && theme !== null) {
      themes.push(theme);
    }
  }
  if (options.preloadLanguages) {
    langs.push(
      ...await Promise.all(
        [...new Set(options.preloadLanguages)].filter((id) => !!bundledLanguages[id]).map(
          (id) => {
            loadedLanguages.add(id);
            return bundledLanguages[id]();
          },
        ),
      ),
    );
  }
  const highlighter = await getHighlighterCore({ langs, themes, loadWasm });
  const setupDataMap = Object.fromEntries(
    await Promise.all(
      bundledLanguageIds.filter((id) => !!lspIndex[id]?.api).map(async (id) => {
        const { init } = await import(
          new URL(`./lsp/${lspIndex[id].id}/api.js`, import.meta.url).href
        );
        return [id, init(monaco, id)];
      }),
    ),
  );
  for (const id of bundledLanguageIds) {
    monaco.languages.register({ id });
    monaco.languages.onLanguage(id, async () => {
      if (!loadedLanguages.has(id)) {
        loadedLanguages.add(id);
        highlighter.loadLanguage(bundledLanguages[id]()).then(() => {
          // activate the highlighter for the language
          shikiToMonaco(highlighter, monaco);
        });
      }
      if (lspIndex[id]) {
        const { setup } = await import(
          new URL(`./lsp/${lspIndex[id].id}/setup.js`, import.meta.url)
            .href
        );
        setup(id, monaco, setupDataMap[id]);
      }
    });
  }
  shikiToMonaco(highlighter, monaco);
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
