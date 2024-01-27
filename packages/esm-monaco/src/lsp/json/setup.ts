import * as monacoNS from "monaco-editor-core";
import * as ls from "../ls-types";
import { CreateData, JSONWorker } from "./worker";

export function setup(languageId: string, monaco: typeof monacoNS) {
  const bus = new monaco.Emitter<void>();
  const languages = monaco.languages;
  const createData: CreateData = {
    languageId,
    options: {
      settings: {
        validate: true,
        allowComments: true,
        schemas: [], // TODO: add built-in schemas?
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
  const workerAccessor: ls.WorkerAccessor<JSONWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<JSONWorker> => {
    return worker.withSyncedResources(uris);
  };

  class JSONDiagnosticsAdapter extends ls.DiagnosticsAdapter<JSONWorker> {
    constructor(
      languageId: string,
      worker: ls.WorkerAccessor<JSONWorker>,
    ) {
      super(languageId, worker, bus.event);
      monaco.editor.onWillDisposeModel((model) => {
        this._resetSchema(model.uri);
      });
      monaco.editor.onDidChangeModelLanguage((event) => {
        this._resetSchema(event.model.uri);
      });
    }

    private _resetSchema(resource: monacoNS.Uri): void {
      this._worker().then((worker) => {
        worker.resetSchema(resource.toString());
      });
    }
  }

  ls.preclude(monaco);
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new ls.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new ls.DocumentRangeFormattingEditProvider(workerAccessor),
  );
  languages.registerCompletionItemProvider(
    languageId,
    new ls.CompletionAdapter(workerAccessor, [" ", ":", '"']),
  );
  languages.registerHoverProvider(
    languageId,
    new ls.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new ls.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerColorProvider(
    languageId,
    new ls.DocumentColorAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new ls.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new ls.SelectionRangeAdapter(workerAccessor),
  );
  new JSONDiagnosticsAdapter(languageId, workerAccessor);
}
