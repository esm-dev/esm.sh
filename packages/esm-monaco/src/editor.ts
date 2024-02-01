import * as monaco from "monaco-editor-core";
import lspIndex from "./lsp/index";
import { allThemes, getLanguageIdFromExtension, initShiki } from "./shiki";
import { VFS } from "./vfs";

let defaultTheme = "vitesse-dark";

interface InitOptions {
  vfs?: VFS;
  themes?: (string | { name: string })[];
  preloadGrammars?: string[];
  customGrammars?: { name: string }[];
}

export async function init(options: InitOptions = {}) {
  Reflect.set(globalThis, "MonacoEnvironment", {
    getWorker: async (_workerId: string, label: string) => {
      let url = new URL("./editor-worker.js", import.meta.url);
      let lsp = lspIndex[label];
      if (!lsp) {
        lsp = Object.values(lspIndex).find((lsp) => lsp.alias?.includes(label));
      }
      if (lsp) {
        url = (await (lsp.import())).workerUrl();
      }
      if (url.hostname === "esm.sh") {
        const { default: workerFactory } = await import(url.href + "?worker");
        return workerFactory();
      }
      return new Worker(url, { type: "module" });
    },
  });

  let themes = (options.themes || [defaultTheme]).filter((v) =>
    (typeof v === "string" && allThemes.some((t) => t.name === v)) ||
    (typeof v === "object" && v !== null && v.name)
  );
  if (themes.length > 0) {
    const theme = themes[0];
    defaultTheme = typeof theme === "string" ? theme : theme.name;
  } else {
    themes = [defaultTheme];
  }

  if (options.vfs) {
    Reflect.set(monaco.editor, "vfs", options.vfs);
    try {
      const list = await options.vfs.list();
      for (const path of list) {
        const lang = getLanguageIdFromExtension(path);
        const preloadGrammars = options.preloadGrammars ??
          (options.preloadGrammars = []);
        if (lang && !preloadGrammars.includes(lang)) {
          preloadGrammars.push(lang);
        }
      }
    } catch {
      // ignore
    }
  }

  await initShiki(monaco, {
    ...options,
    themes,
    onLanguage: async (id: string) => {
      let lsp = lspIndex[id];
      if (!lsp) {
        lsp = Object.values(lspIndex).find((lsp) => lsp.alias?.includes(id));
      }
      if (lsp) {
        (await lsp.import()).setup(id, monaco);
      }
    },
  });

  customElements.define(
    "monaco-editor",
    class extends HTMLElement {
      #editor: monaco.editor.IStandaloneCodeEditor;
      #text: string;

      get editor() {
        return this.#editor;
      }

      constructor() {
        super();
        const options: monaco.editor.IStandaloneEditorConstructionOptions = {};
        const optionKeys = [
          "autoDetectHighContrast",
          "automaticLayout",
          "contextmenu",
          "cursorBlinking",
          "detectIndentation",
          "fontFamily",
          "fontLigatures",
          "fontSize",
          "fontVariations",
          "fontWeight",
          "insertSpaces",
          "letterSpacing",
          "lineHeight",
          "lineNumbers",
          "linkedEditing",
          "minimap",
          "padding",
          "readOnly",
          "rulers",
          "scrollbar",
          "smoothScrolling",
          "tabIndex",
          "tabSize",
          "theme",
          "trimAutoWhitespace",
          "wordWrap",
          "wordWrapColumn",
        ];
        for (const { name: attrName, value: attrVar } of this.attributes) {
          const key = optionKeys.find((k) => k.toLowerCase() === attrName);
          if (key) {
            let value: any = attrVar;
            if (value === "") {
              value = attrName === "minimap" ? { enabled: true } : true;
            } else {
              try {
                value = JSON.parse(value);
              } catch {
                // ignore
              }
            }
            options[key] = value;
          }
        }
        this.style.display = "block";
        this.#text = this.textContent;
        this.replaceChildren()
        this.#editor = monaco.editor.create(this, options);
      }
      async connectedCallback() {
        const file = this.getAttribute("file");
        const language = this.getAttribute("language");
        if (file && options.vfs) {
          this.#editor.setModel(await options.vfs.openModel(file));
        } else {
          this.#editor.setModel(
            monaco.editor.createModel(
              this.#text,
              this.getAttribute("language"),
            ),
          );
        }
      }
    },
  );
}

const _create = monaco.editor.create;
const _createModel = monaco.editor.createModel;

// override default create and createModel methods
Object.assign(monaco.editor, {
  create: (
    container: HTMLElement,
    options?: monaco.editor.IStandaloneEditorConstructionOptions,
  ): monaco.editor.IStandaloneCodeEditor => {
    return _create(
      container,
      {
        automaticLayout: true,
        minimap: { enabled: false },
        theme: defaultTheme,
        ...options,
      } satisfies typeof options,
    );
  },
  createModel: (
    value: string,
    language?: string,
    uri?: string | URL | monaco.Uri,
  ) => {
    if (typeof uri === "string" || uri instanceof URL) {
      const url = new URL(uri, "file:///");
      uri = monaco.Uri.parse(url.href);
    }
    if (!language && uri) {
      language = getLanguageIdFromExtension(uri.path);
    }
    return _createModel(value, language, uri);
  },
});

export * from "monaco-editor-core";
export * from "./vfs";
