import type * as monacoNS from "monaco-editor-core";
import { EventTrigger, SetupData } from "./api";
import type {
  CreateData,
  DiagnosticsOptions,
  Host,
  TypeScriptWorker,
} from "./worker";
import * as lf from "./language-features";

// javascript and typescript share the same worker
let worker:
  | monacoNS.editor.MonacoWebWorker<TypeScriptWorker>
  | Promise<monacoNS.editor.MonacoWebWorker<TypeScriptWorker>>
  | null = null;
let refreshDiagnosticEventEmitter: EventTrigger | null = null;

export async function setup(
  languageId: string,
  monaco: typeof monacoNS,
  data: SetupData,
) {
  const languages = monaco.languages;
  const {
    compilerOptions,
    importMap,
    libFiles,
    onCompilerOptionsChange,
    onExtraLibsChange,
  } = data;

  if (!worker) {
    worker = (async () => {
      libFiles.setLibs(
        (await import(new URL("./libs.js", import.meta.url).href)).default,
      );
      const createData: CreateData = {
        importMap,
        compilerOptions,
        libs: libFiles.libs,
        extraLibs: libFiles.extraLibs,
      };
      return monaco.editor.createWebWorker<TypeScriptWorker>({
        moduleId: "lsp/typescript/worker",
        label: languageId,
        keepIdleModels: true,
        createData,
        host: {
          tryOpenModel: async (uri: string): Promise<boolean> => {
            try {
              // @ts-expect-error the `openModel` method is added by esm-monaco
              monaco.editor.openModel(uri);
            } catch (error) {
              // @ts-expect-error the `vfs` member is added by esm-monaco
              if (error instanceof monaco.editor.vfs.ErrorNotFound) {
                return false;
              }
            }
            return true; // model is opened or error is not NotFound
          },
          refreshDiagnostics: async () => {
            refreshDiagnosticEventEmitter.fire();
          },
        } satisfies Host,
      });
    })();
    refreshDiagnosticEventEmitter = new EventTrigger(new monaco.Emitter());
  }
  if (worker instanceof Promise) {
    worker = await worker;
  }

  const workerWithResources = (
    ...uris: monacoNS.Uri[]
  ): Promise<TypeScriptWorker> => {
    return (worker as monacoNS.editor.MonacoWebWorker<TypeScriptWorker>)
      .withSyncedResources(uris);
  };

  // tell the worker to update the compiler options when it changes.
  let updateCompilerOptionsToken = 0;
  onCompilerOptionsChange(async () => {
    const myToken = ++updateCompilerOptionsToken;
    const proxy =
      await (worker as monacoNS.editor.MonacoWebWorker<TypeScriptWorker>)
        .getProxy();
    if (updateCompilerOptionsToken !== myToken) {
      // avoid multiple calls
      return;
    }
    proxy.updateCompilerOptions(compilerOptions, importMap);
  });

  // tell the worker to update the extra libs when it changes.
  let updateExtraLibsToken = 0;
  onExtraLibsChange(async () => {
    const myToken = ++updateExtraLibsToken;
    const proxy =
      await (worker as monacoNS.editor.MonacoWebWorker<TypeScriptWorker>)
        .getProxy();
    if (updateExtraLibsToken !== myToken) {
      // avoid multiple calls
      return;
    }
    proxy.updateExtraLibs(libFiles.extraLibs);
  });

  // register language features
  lf.preclude(monaco);
  languages.registerCompletionItemProvider(
    languageId,
    new lf.SuggestAdapter(workerWithResources),
  );
  languages.registerSignatureHelpProvider(
    languageId,
    new lf.SignatureHelpAdapter(workerWithResources),
  );
  languages.registerHoverProvider(
    languageId,
    new lf.QuickInfoAdapter(workerWithResources),
  );
  languages.registerDocumentHighlightProvider(
    languageId,
    new lf.DocumentHighlightAdapter(workerWithResources),
  );
  languages.registerDefinitionProvider(
    languageId,
    new lf.DefinitionAdapter(libFiles, workerWithResources),
  );
  languages.registerReferenceProvider(
    languageId,
    new lf.ReferenceAdapter(libFiles, workerWithResources),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lf.OutlineAdapter(workerWithResources),
  );
  languages.registerRenameProvider(
    languageId,
    new lf.RenameAdapter(libFiles, workerWithResources),
  );
  languages.registerDocumentRangeFormattingEditProvider(
    languageId,
    new lf.FormatAdapter(workerWithResources),
  );
  languages.registerOnTypeFormattingEditProvider(
    languageId,
    new lf.FormatOnTypeAdapter(workerWithResources),
  );
  languages.registerCodeActionProvider(
    languageId,
    new lf.CodeActionAdaptor(workerWithResources),
  );
  languages.registerInlayHintsProvider(
    languageId,
    new lf.InlayHintsAdapter(workerWithResources),
  );

  const diagnosticsOptions: DiagnosticsOptions = {
    noSemanticValidation: languageId === "javascript",
    noSyntaxValidation: false,
    onlyVisible: false,
  };
  new lf.DiagnosticsAdapter(
    libFiles,
    diagnosticsOptions,
    [
      onExtraLibsChange,
      onCompilerOptionsChange,
      refreshDiagnosticEventEmitter.event,
    ],
    languageId,
    workerWithResources,
  );
}
