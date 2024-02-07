import type * as monacoNS from "monaco-editor-core";
import type { HighlighterCore } from "@shikijs/core";
import { shikiToMonaco } from "@shikijs/monaco";
import type { ShikiInitOptions } from "./shiki";
import { getLanguageIdFromPath, initShiki } from "./shiki";
import { grammarRegistry, loadedGrammars, loadTMGrammer } from "./shiki";
import { render, type RenderOptions } from "./render.js";
import { VFS } from "./vfs";
import lspIndex from "./lsp/index";

const editorOptionKeys = [
  "autoDetectHighContrast",
  "automaticLayout",
  "contextmenu",
  "cursorBlinking",
  "detectIndentation",
  "extraEditorClassName",
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

  if (vfs) {
    vfs.bindMonaco(monaco);
  }

  for (const id of grammarRegistry) {
    monaco.languages.register({ id });
    monaco.languages.onLanguage(id, () => {
      if (!loadedGrammars.has(id)) {
        loadedGrammars.add(id);
        highlighter.loadLanguage(loadTMGrammer(id)).then(() => {
          // activate the highlighter for the language
          shikiToMonaco(highlighter, monaco);
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
          // ignore vfs error
        }
      }
    };
    loading = getGrammarsInVFS().then(() =>
      initShiki(options).then((shiki) => loadEditor(shiki, options.vfs))
    );
  }
  return loading;
}

export function lazyMode(options: InitOption = {}) {
  customElements.define(
    "monaco-editor",
    class extends HTMLElement {
      #editor: monacoNS.editor.IStandaloneCodeEditor;
      #vfs?: VFS;

      get editor() {
        return this.#editor;
      }

      constructor() {
        super();
        this.style.display = "block";
        this.style.position = "relative";
        this.#vfs = options.vfs;
      }

      async connectedCallback() {
        const renderOptions: Partial<RenderOptions> = {};

        // check editor/render options from attributes
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
            renderOptions[key] = value;
          }
        }

        // check editor options from the first script child
        const optionsScript = this.children[0] as HTMLScriptElement | null;
        if (
          optionsScript &&
          optionsScript.tagName === "SCRIPT" &&
          optionsScript.type === "application/json"
        ) {
          const options = JSON.parse(optionsScript.textContent);
          // we pass the `fontMaxDigitWidth` option to the editor as a
          // custom class name. this is used for keeping the line numbers
          // layout consistent between the SSR render and the client pre-render.
          if (options.fontMaxDigitWidth) {
            options.extraEditorClassName = [
              options.extraEditorClassName,
              "font-max-digit-width-" +
              options.fontMaxDigitWidth.toString().replace(".", "_"),
            ].filter(Boolean).join(" ");
          }
          Object.assign(renderOptions, options);
          optionsScript.remove();
        }

        // set dimension from width and height attributes
        const width = Number(this.getAttribute("width"));
        const height = Number(this.getAttribute("height"));
        if (width > 0 && height > 0) {
          this.style.width = `${width}px`;
          this.style.height = `${height}px`;
          renderOptions.dimension = { width, height };
        }

        // the container element for monaco editor instance
        const containerEl = document.createElement("div");
        containerEl.className = "monaco-editor-container";
        containerEl.style.width = "100%";
        containerEl.style.height = "100%";
        this.insertBefore(containerEl, this.firstChild);

        // crreate a highlighter instance for the renderer/editor
        const preloadGrammars = options.preloadGrammars ?? [];
        const file = renderOptions.filename ?? this.getAttribute("file");
        if (renderOptions.lang || file) {
          preloadGrammars.push(
            renderOptions.lang ?? getLanguageIdFromPath(file),
          );
        }
        const highlighter = await initShiki({ ...options, preloadGrammars });
        const vfs = this.#vfs;

        // check the pre-rendered content, if not exists, render one
        let preRenderEl = this.querySelector<HTMLElement>(
          ".monaco-editor-prerender",
        );
        if (
          !preRenderEl &&
          ((file && vfs) || (renderOptions.code && renderOptions.lang))
        ) {
          let code = renderOptions.code;
          let lang = renderOptions.lang;
          if (vfs && file) {
            code = await vfs.readTextFile(file);
            lang = getLanguageIdFromPath(file);
          }
          preRenderEl = containerEl.cloneNode(true) as HTMLElement;
          preRenderEl.className = "monaco-editor-prerender";
          preRenderEl.style.position = "absolute";
          preRenderEl.style.top = "0";
          preRenderEl.style.left = "0";
          preRenderEl.innerHTML = render(highlighter, {
            ...renderOptions,
            lang,
            code,
          });
          this.appendChild(preRenderEl);
        }

        // add a transition effect to hide the pre-rendered content
        if (preRenderEl) {
          preRenderEl.style.opacity = "1";
          preRenderEl.style.transition = "opacity 0.3s";
        }

        // load monaco editor
        loadEditor(highlighter, vfs).then(async (monaco) => {
          this.#editor = monaco.editor.create(containerEl, renderOptions);
          if (vfs && file) {
            const model = await vfs.openModel(file);
            if (
              renderOptions.filename === file &&
              renderOptions.code &&
              renderOptions.code !== model.getValue()
            ) {
              // update the model value with the code from SSR
              model.setValue(renderOptions.code);
            }
            this.#editor.setModel(model);
          } else if ((renderOptions.code && renderOptions.lang)) {
            const model = monaco.editor.createModel(
              renderOptions.code,
              renderOptions.lang,
              // @ts-expect-error the overwrited `createModel` method supports
              // path as the third argument(URI)
              renderOptions.filename,
            );
            this.#editor.setModel(model);
          }
          // hide the prerender element if exists
          if (preRenderEl) {
            setTimeout(() => {
              preRenderEl.style.opacity = "0";
              setTimeout(() => {
                preRenderEl.remove();
              }, 300);
            }, 500);
          }
        });
      }
    },
  );
}

export async function renderToString(options: RenderOptions) {
  if (options.filename && !options.lang) {
    options.lang = getLanguageIdFromPath(options.filename);
  }
  const highlighter = await (ssrHighlighter ?? (ssrHighlighter = initShiki({
    theme: options.theme,
    preloadGrammars: [options.lang],
  })));
  if (!loadedGrammars.has(options.lang)) {
    loadedGrammars.add(options.lang);
    await highlighter.loadLanguage(loadTMGrammer(options.lang));
  }
  return [
    `<monaco-editor>`,
    `<script type="application/json" class="monaco-editor-options">${
      JSON.stringify(options)
    }</script>`,
    `<div class="monaco-editor-prerender" style="width:100%;height:100%;position:absolute;top:0px;left:0px">`,
    render(highlighter, options),
    `</div>`,
    `</monaco-editor>`,
  ].join("");
}

export { VFS };
