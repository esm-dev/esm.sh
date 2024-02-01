import type * as monacoNS from "monaco-editor-core";
import * as lf from "../language-features";
import type { CreateData, JSONWorker } from "./worker";
import { schemas } from "./schemas";

export function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;
  const events = new monaco.Emitter<void>();
  const createData: CreateData = {
    languageId,
    options: {
      settings: {
        validate: true,
        allowComments: false,
        schemas,
        schemaRequest: "warning",
        schemaValidation: "warning",
        comments: "error",
        trailingCommas: "error",
      },
      format: {
        tabSize: 4,
        insertSpaces: false,
        trimTrailingWhitespace: true,
        insertFinalNewline: true,
      },
    },
  };
  const worker = monaco.editor.createWebWorker<JSONWorker>({
    moduleId: "lsp/json/worker",
    label: languageId,
    createData,
  });
  const workerAccessor: lf.WorkerAccessor<JSONWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<JSONWorker> => {
    return worker.withSyncedResources(uris);
  };

  class JSONDiagnosticsAdapter extends lf.DiagnosticsAdapter<JSONWorker> {
    constructor(
      languageId: string,
      worker: lf.WorkerAccessor<JSONWorker>,
    ) {
      super(languageId, worker, events.event);
      const editor = monaco.editor;
      editor.onWillDisposeModel((model) => {
        this._resetSchema(model.uri);
      });
      editor.onDidChangeModelLanguage((event) => {
        this._resetSchema(event.model.uri);
      });
    }

    private _resetSchema(resource: monacoNS.Uri): void {
      this._worker().then((worker) => {
        worker.resetSchema(resource.toString());
      });
    }
  }

  lf.preclude(monaco);
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new lf.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new lf.DocumentRangeFormattingEditProvider(workerAccessor),
  );
  languages.registerCompletionItemProvider(
    languageId,
    new lf.CompletionAdapter(workerAccessor, [" ", ":", '"']),
  );
  languages.registerHoverProvider(
    languageId,
    new lf.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lf.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerColorProvider(
    languageId,
    new lf.DocumentColorAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new lf.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new lf.SelectionRangeAdapter(workerAccessor),
  );
  new JSONDiagnosticsAdapter(languageId, workerAccessor);
}

export function workerUrl() {
  return new URL("worker.js", import.meta.url).href;
}
