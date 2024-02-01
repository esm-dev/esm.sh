import type * as monacoNS from "monaco-editor-core";
import type ts from "typescript";
import type { VFS } from "../../vfs";
import type { CreateData, Host, ImportMap, TypeScriptWorker } from "./worker";
import * as lf from "./language-features";

// javascript and typescript share the same worker
let worker:
  | monacoNS.editor.MonacoWebWorker<TypeScriptWorker>
  | Promise<monacoNS.editor.MonacoWebWorker<TypeScriptWorker>>
  | null = null;
let refreshDiagnosticEventEmitter: EventTrigger | null = null;

class EventTrigger {
  private _fireTimer = null;

  constructor(private _emitter: monacoNS.Emitter<void>) {}

  public get event() {
    return this._emitter.event;
  }

  public fire() {
    if (this._fireTimer !== null) {
      // already fired
      return;
    }
    this._fireTimer = setTimeout(() => {
      this._fireTimer = null;
      this._emitter.fire();
    }, 0) as any;
  }
}

async function createWorker(monaco: typeof monacoNS) {
  const compilerOptions: ts.CompilerOptions = {
    allowImportingTsExtensions: true,
    allowJs: true,
    module: 99, // ModuleKind.ESNext,
    moduleResolution: 100, // ModuleResolutionKind.Bundler,
    target: 99, // ScriptTarget.ESNext,
    noEmit: true,
  };
  const importMap: ImportMap = {};
  const vfs = Reflect.get(monaco.editor, "vfs") as VFS | undefined;
  const libsPromise = import("./libs.js").then((m) => m.default);

  if (vfs) {
    try {
      const tconfigjson = await vfs.readTextFile("tsconfig.json");
      const tconfig = JSON.parse(tconfigjson);
      const types = tconfig.compilerOptions.types;
      delete tconfig.compilerOptions.types;
      if (Array.isArray(types)) {
        for (const type of types) {
          // TODO: support type from http
          try {
            const dts = await vfs.readTextFile(type);
            lf.libFiles.addExtraLib(dts, type);
          } catch (error) {
            if (error instanceof vfs.ErrorNotFound) {
              // ignore
            } else {
              console.error(error);
            }
          }
        }
      }
      Object.assign(compilerOptions, tconfig.compilerOptions);
    } catch (error) {
      if (error instanceof vfs.ErrorNotFound) {
        // ignore
      } else {
        console.error(error);
      }
    }
  }
  // todo: watch tsconfig.json

  const libs = await libsPromise;
  const createData: CreateData = {
    compilerOptions,
    libs,
    extraLibs: lf.libFiles.extraLibs,
    importMap,
  };
  lf.libFiles.setLibs(libs);
  return monaco.editor.createWebWorker<TypeScriptWorker>({
    moduleId: "lsp/typescript/worker",
    label: "typescript",
    keepIdleModels: true,
    createData,
    host: {
      tryOpenModel: async (uri: string): Promise<boolean> => {
        const vfs = Reflect.get(monaco.editor, "vfs") as VFS | undefined;
        if (!vfs) {
          return false; // vfs is not enabled
        }
        try {
          await vfs.openModel(uri);
        } catch (error) {
          if (error instanceof vfs.ErrorNotFound) {
            return false;
          }
        }
        return true; // model is opened or error is not NotFound
      },
      refreshDiagnostics: async () => {
        refreshDiagnosticEventEmitter?.fire();
      },
    } satisfies Host,
  });
}

export async function setup(languageId: string, monaco: typeof monacoNS) {
  const languages = monaco.languages;

  if (!refreshDiagnosticEventEmitter) {
    refreshDiagnosticEventEmitter = new EventTrigger(new monaco.Emitter());
  }

  if (!worker) {
    worker = createWorker(monaco);
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
    new lf.DefinitionAdapter(workerWithResources),
  );
  languages.registerReferenceProvider(
    languageId,
    new lf.ReferenceAdapter(workerWithResources),
  );
  languages.registerDocumentSymbolProvider(
    languageId,
    new lf.OutlineAdapter(workerWithResources),
  );
  languages.registerRenameProvider(
    languageId,
    new lf.RenameAdapter(workerWithResources),
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

  const diagnosticsOptions: lf.DiagnosticsOptions = {
    noSemanticValidation: languageId === "javascript",
    noSyntaxValidation: false,
    onlyVisible: false,
  };
  new lf.DiagnosticsAdapter(
    diagnosticsOptions,
    refreshDiagnosticEventEmitter.event,
    languageId,
    workerWithResources,
  );
}

export function workerUrl() {
  return new URL("worker.js", import.meta.url).href;
}
