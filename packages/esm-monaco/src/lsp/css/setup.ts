import * as monacoNS from "monaco-editor-core";
import type { CreateData, CSSWorker } from "./worker";
import * as lsTypes from "../ls-types";

export function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;
  const bus = new monaco.Emitter<void>();
  const createData: CreateData = {
    languageId,
    options: {
      data: {
        useDefaultDataProvider: true,
      },
      format: {
        newlineBetweenSelectors: true,
        newlineBetweenRules: true,
        spaceAroundSelectorSeparator: false,
        braceStyle: "collapse",
        preserveNewLines: true,
      },
    },
  };
  const worker = monaco.editor.createWebWorker<CSSWorker>({
    moduleId: "lsp/css/worker",
    label: languageId,
    createData,
  });
  const workerAccessor: lsTypes.WorkerAccessor<CSSWorker> = (
    ...uris: monacoNS.Uri[]
  ): Promise<CSSWorker> => {
    return worker.withSyncedResources(uris);
  };

  lsTypes.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new lsTypes.CompletionAdapter(workerAccessor, ["/", "-", ":"]),
  );
  languages.registerHoverProvider(
    languageId,
    new lsTypes.HoverAdapter(workerAccessor),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new lsTypes.DocumentHighlightAdapter(workerAccessor),
  );
  languages.registerDefinitionProvider(
    languageId,
    new lsTypes.DefinitionAdapter(workerAccessor),
  );
  languages.registerReferenceProvider(
    languageId,
    new lsTypes.ReferenceAdapter(workerAccessor),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lsTypes.DocumentSymbolAdapter(workerAccessor),
  );
  languages.registerRenameProvider(
    languageId,
    new lsTypes.RenameAdapter(workerAccessor),
  );
  languages.registerColorProvider(
    languageId,
    new lsTypes.DocumentColorAdapter(workerAccessor),
  );
  languages.registerFoldingRangeProvider(
    languageId,
    new lsTypes.FoldingRangeAdapter(workerAccessor),
  );
  languages.registerSelectionRangeProvider(
    languageId,
    new lsTypes.SelectionRangeAdapter(workerAccessor),
  );
  languages.registerDocumentFormattingEditProvider(
    languageId,
    new lsTypes.DocumentFormattingEditProvider(workerAccessor),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new lsTypes.DocumentRangeFormattingEditProvider(workerAccessor),
  );
  new lsTypes.DiagnosticsAdapter(
    languageId,
    workerAccessor,
    bus.event,
  );
}
