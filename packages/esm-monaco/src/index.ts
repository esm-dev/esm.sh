import type * as monacoNS from "monaco-editor-core";
import type { HighlighterCore } from "@shikijs/core";
import { shikiToMonaco } from "@shikijs/monaco";
import type { ShikiInitOptions } from "./shiki";
import { getLanguageIdFromPath, initShiki } from "./shiki";
import { loadedGrammars, loadTMGrammer, tmGrammerRegistry } from "./shiki";
import { render, type RenderOptions } from "./render.js";
import { VFS } from "./vfs";
import lspIndex from "./lsp/index";

const editorOptionKeys = [
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
  "wrappingIndent",
];

export interface InitOption extends ShikiInitOptions {
  vfs?: VFS;
}

async function loadEditor(highlighter: HighlighterCore, vfs?: VFS) {
  const monaco = await import("./editor.js");
  const editorWorkerUrl = monaco.workerUrl();

  if (vfs) {
    vfs.bindMonaco(monaco);
  }

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
    getLanguageIdFromUri: (uri: monacoNS.Uri) =>
      getLanguageIdFromPath(uri.path),
  });

  if (!document.getElementById("monaco-editor-core-css")) {
    const styleEl = document.createElement("style");
    styleEl.id = "monaco-editor-core-css";
    styleEl.media = "screen";
    // @ts-expect-error `_CSS` is defined at build time
    styleEl.textContent = monaco._CSS;
    document.head.appendChild(styleEl);
  }

  for (const id of tmGrammerRegistry) {
    monaco.languages.register({ id });
    monaco.languages.onLanguage(id, () => {
      if (!loadedGrammars.has(id)) {
        loadedGrammars.add(id);
        highlighter.loadLanguage(loadTMGrammer(id)).then(() => {
          // activate the highlighter for the language
          shikiToMonaco(highlighter, monaco);
          console.log(`[monaco-shiki] grammar "${id}" loaded.`);
        });
      }
      let lsp = lspIndex[id];
      if (!lsp) {
        lsp = Object.values(lspIndex).find((lsp) => lsp.aliases?.includes(id));
      }
      if (lsp) {
        lsp.import().then(({ setup }) => setup(id, monaco, vfs));
      }
    });
  }
  shikiToMonaco(highlighter, monaco);

  return monaco;
}

let loading: Promise<typeof monacoNS> | undefined;
let ssrHighlighter: HighlighterCore | Promise<HighlighterCore> | undefined;

export function init(options: InitOption = {}) {
  if (!loading) {
    const getGrammarsInVFS = async () => {
      const vfs = options.vfs;
      const preloadGrammars = options.preloadGrammars ?? [];
      if (vfs) {
        try {
          const list = await vfs.list();
          for (const path of list) {
            const lang = getLanguageIdFromPath(path);
            if (lang && !preloadGrammars.includes(lang)) {
              preloadGrammars.push(lang);
            }
          }
          options.preloadGrammars = preloadGrammars;
        } catch {
          // ignore vsf error
        }
      }
    };
    loading = getGrammarsInVFS().then(() =>
      initShiki(options).then((shiki) => loadEditor(shiki, options.vfs))
    );
  }
  return loading;
}

export function lazyMode(options: InitOption & { ssr?: boolean } = {}) {
  customElements.define(
    "monaco-editor",
    class extends HTMLElement {
      #editor: monacoNS.editor.IStandaloneCodeEditor;

      get editor() {
        return this.#editor;
      }

      async connectedCallback() {
        const createOptions:
          monacoNS.editor.IStandaloneEditorConstructionOptions = {};
        for (const attrName of this.getAttributeNames()) {
          const key = editorOptionKeys.find((k) =>
            k.toLowerCase() === attrName
          );
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
            if (key === "padding" && typeof value === "number") {
              value = { top: value, bottom: value };
            }
            createOptions[key] = value;
          }
        }
        const width = parseInt(this.getAttribute("width"));
        const height = parseInt(this.getAttribute("height"));
        if (width > 0 && height > 0) {
          this.style.width = `${width}px`;
          this.style.height = `${height}px`;
          createOptions.dimension = { width, height };
        }
        this.style.position = "relative";
        this.style.display = "block";

        const containerEl = document.createElement("div");
        containerEl.className = "monaco-editor-container";
        containerEl.style.width = "100%";
        containerEl.style.height = "100%";
        this.appendChild(containerEl);

        let placeHolderEl: HTMLElement | null = null;
        if (options.ssr) {
          placeHolderEl = this.querySelector(".monaco-editor-container");
          this.appendChild(placeHolderEl); // move to the end
        } else {
          placeHolderEl = containerEl.cloneNode(true) as HTMLElement;
          placeHolderEl.style.position = "absolute";
          placeHolderEl.style.top = "0";
          placeHolderEl.style.left = "0";
          this.appendChild(placeHolderEl);
        }

        const file = this.getAttribute("file");
        const preloadGrammars = options.preloadGrammars ?? [];
        if (file) {
          preloadGrammars.push(getLanguageIdFromPath(file));
        }
        const highlighter = await initShiki({ ...options, preloadGrammars });
        const vfs = options.vfs;
        if (vfs && file && !options.ssr) {
          placeHolderEl.innerHTML = render(
            highlighter,
            {
              code: await vfs.readTextFile(file),
              lang: getLanguageIdFromPath(file),
              ...createOptions,
            },
          );
        }
        if (placeHolderEl) {
          placeHolderEl.style.opacity = "1";
          placeHolderEl.style.transition = "opacity 0.3s";
        }
        loadEditor(highlighter, vfs).then(async (monaco) => {
          this.#editor = monaco.editor.create(containerEl, createOptions);
          const file = this.getAttribute("file");
          if (file && vfs) {
            const model = await vfs.openModel(file);
            this.#editor.setModel(model);
            if (placeHolderEl) {
              setTimeout(() => {
                placeHolderEl.style.opacity = "0";
                setTimeout(() => {
                  placeHolderEl.remove();
                }, 300);
              }, 500);
            }
          }
        });
      }
    },
  );
}

export async function renderToString(
  options: RenderOptions & { filename?: string; theme?: string },
) {
  const attrs = [];
  if (options.filename) {
    attrs.push(`file="${options.filename}"`);
    if (!options.lang) {
      options.lang = getLanguageIdFromPath(options.filename);
    }
  }
  const highlighter = await (ssrHighlighter ?? (ssrHighlighter = initShiki({
    preloadGrammars: [options.lang],
    theme: options.theme,
  })));
  if (!loadedGrammars.has(options.lang)) {
    highlighter.loadLanguage(loadTMGrammer(options.lang));
  }
  for (const key of Object.keys(options)) {
    if (editorOptionKeys.includes(key)) {
      let value = options[key];
      if (value !== undefined && value !== null) {
        if (typeof value !== "string") {
          value = JSON.stringify(value);
        }
        attrs.push(`${key}='${value}'`);
      }
    }
  }
  return [
    `<monaco-editor ${attrs.join(" ")}>`,
    `<div class="monaco-editor-container" style="width: 100%; height: 100%; position: absolute; top: 0px; left: 0px;">`,
    render(highlighter, options),
    `</div>`,
    `</monaco-editor>`,
  ].join("");
}

export { VFS };
