import * as monacoNS from "monaco-editor-core";
import * as lsTypes from "../ls-types";
import type { CreateData, HTMLWorker } from "./worker";

class HTMLCompletionAdapter extends lsTypes.CompletionAdapter<HTMLWorker> {
  constructor(worker: lsTypes.WorkerAccessor<HTMLWorker>) {
    super(worker, [".", ":", "<", '"', "=", "/"]);
  }
}

export function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;
  const createData: CreateData = {
    languageId,
    options: {
      data: {
        useDefaultDataProvider: true,
      },
      suggest: {},
      format: {
        tabSize: 4,
        insertSpaces: false,
        wrapLineLength: 120,
        unformatted:
          'default": "a, abbr, acronym, b, bdo, big, br, button, cite, code, dfn, em, i, img, input, kbd, label, map, object, q, samp, select, small, span, strong, sub, sup, textarea, tt, var',
        contentUnformatted: "pre",
        indentInnerHtml: false,
        preserveNewLines: true,
        maxPreserveNewLines: undefined,
        indentHandlebars: false,
        endWithNewline: false,
        extraLiners: "head, body, /html",
        wrapAttributes: "auto",
      },
    },
  };
  const worker = monaco.editor.createWebWorker<HTMLWorker>({
    moduleId: "lsp/css/worker",
    label: languageId,
    createData,
  });
  const workerAccessor: lsTypes.WorkerAccessor<HTMLWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<HTMLWorker> => {
    return worker.withSyncedResources(uris);
  };

  lsTypes.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new HTMLCompletionAdapter(workerAccessor),
  );
  languages.registerHoverProvider(
    languageId,
    new lsTypes.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new lsTypes.DocumentHighlightAdapter(workerAccessor),
  );
  languages.registerLinkProvider(
    languageId,
    new lsTypes.DocumentLinkAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new lsTypes.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lsTypes.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new lsTypes.SelectionRangeAdapter(workerAccessor),
  );
  languages.registerRenameProvider(
    languageId,
    new lsTypes.RenameAdapter(workerAccessor),
  );
  // only html
  if (languageId === "html") {
    languages.registerDocumentFormattingEditProvider(
      languageId,
      new lsTypes.DocumentFormattingEditProvider(workerAccessor),
    );
    languages.registerDocumentRangeFormattingEditProvider(
      languageId,
      new lsTypes.DocumentRangeFormattingEditProvider(workerAccessor),
    );
  }
}
