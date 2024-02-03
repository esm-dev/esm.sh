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
  const editorWorkerUrl = workerUrl();

  Reflect.set(globalThis, "MonacoEnvironment", {
    getWorker: async (_workerId: string, label: string) => {
      let url = editorWorkerUrl;
      let lsp = lspIndex[label];
      if (!lsp) {
        lsp = Object.values(lspIndex).find((lsp) =>
          lsp.aliases?.includes(label)
        );
      }
      if (lsp) {
        url = (await (lsp.import())).workerUrl();
      }
      if (url.hostname === "esm.sh") {
        const { default: workerFactory } = await import(
          url.href.replace(/\.js$/, ".bundle.js") + "?worker"
        );
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
        lsp = Object.values(lspIndex).find((lsp) => lsp.aliases?.includes(id));
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
      #textContent: string;

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
        for (const attrName of this.getAttributeNames()) {
          const key = optionKeys.find((k) => k.toLowerCase() === attrName);
          if (key) {
            let value: any = this.getAttribute(attrName);
            if (value === "") {
              value = attrName === "minimap" ? { enabled: true } : true;
            } else {
              try {
                value = JSON.parse(value);
              } catch {
                // ignore
              }
            }
            if (key === "padding") {
              value = { top: value, bottom: value };
            }
            options[key] = value;
          }
        }
        const width = parseInt(this.getAttribute("width"));
        const height = parseInt(this.getAttribute("height"));
        if (width > 0 && height > 0) {
          this.style.width = `${width}px`;
          this.style.height = `${height}px`;
          options.dimension = { width, height };
        }
        this.style.display = "block";
        this.#textContent = this.textContent;
        this.replaceChildren();
        this.#editor = monaco.editor.create(this, options);
      }
      async connectedCallback() {
        const file = this.getAttribute("file");
        if (file && options.vfs) {
          this.#editor.setModel(await options.vfs.openModel(file));
        } else {
          this.#editor.setModel(
            monaco.editor.createModel(
              this.#textContent,
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
        scrollBeyondLastLine: false,
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

function workerUrl() {
  const m = workerUrl.toString().match(/import\(['"](.+?)['"]\)/);
  if (!m) throw new Error("worker url not found");
  const url = new URL(m[1], import.meta.url);
  Reflect.set(url, "import", () => import("./editor-worker.js")); // trick for bundlers
  return url;
}

export * from "monaco-editor-core";
export * from "./vfs";
