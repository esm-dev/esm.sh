import type * as monacoNS from "monaco-editor-core";
import type ts from "typescript";

export interface ExtraLib {
  content: string;
  version: number;
}

export interface ImportMap {
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

export interface SetupData {
  compilerOptions: ts.CompilerOptions;
  importMap: ImportMap;
  libFiles: LibFiles;
  onCompilerOptionsChange: monacoNS.IEvent<void>;
  onExtraLibsChange: monacoNS.IEvent<void>;
}

export class LibFiles {
  private _removedExtraLibs: Record<string, number> = {};

  constructor(
    private _monaco: typeof monacoNS,
    private _libs: Record<string, string> = {},
    private _extraLibs: Record<string, ExtraLib> = {},
  ) {}

  get libs() {
    return this._libs;
  }

  get extraLibs() {
    return this._extraLibs;
  }

  public setLibs(libs: Record<string, string>) {
    this._libs = libs;
  }

  public setExtraLibs(extraLibs: Record<string, string>) {
    const entries = Object.entries(extraLibs);
    for (const [filePath, content] of entries) {
      this.addExtraLib(content, filePath);
    }
  }

  public addExtraLib(content: string, filePath: string): boolean {
    if (
      this._extraLibs[filePath] &&
      this._extraLibs[filePath].content === content
    ) {
      return false;
    }
    let version = 1;
    if (this._removedExtraLibs[filePath]) {
      version = this._removedExtraLibs[filePath] + 1;
    }
    if (this._extraLibs[filePath]) {
      version = this._extraLibs[filePath].version + 1;
    }
    this._extraLibs[filePath] = { content, version };
    return true;
  }

  public removeExtraLib(filePath: string): boolean {
    const lib = this._extraLibs[filePath];
    if (lib) {
      delete this._extraLibs[filePath];
      this._removedExtraLibs[filePath] = lib.version;
      return true;
    }
    return false;
  }

  public isLibFile(uri: monacoNS.Uri | null): boolean {
    if (!uri) {
      return false;
    }
    if (uri.path.indexOf("/lib.") === 0) {
      return uri.path.slice(1) in this._libs;
    }
    return false;
  }

  public getOrCreateModel(fileName: string): monacoNS.editor.ITextModel | null {
    const editor = this._monaco.editor;
    const uri = this._monaco.Uri.parse(fileName);
    const model = editor.getModel(uri);
    if (model) {
      return model;
    }
    if (this.isLibFile(uri)) {
      return editor.createModel(
        this._libs[uri.path.slice(1)],
        "typescript",
        uri,
      );
    }
    const matchedLibFile = this._extraLibs[fileName];
    if (matchedLibFile) {
      return editor.createModel(matchedLibFile.content, "typescript", uri);
    }
    return null;
  }
}

export class EventTrigger {
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

// javascript and typescript share the setup data
let setupData: SetupData | Promise<SetupData> | null = null;

const createSetupData = async (monaco: typeof monacoNS): Promise<SetupData> => {
  const importMap: ImportMap = {};
  const compilerOptions: ts.CompilerOptions = {
    allowImportingTsExtensions: true,
    allowJs: true,
    module: 99, // ModuleKind.ESNext,
    moduleResolution: 100, // ModuleResolutionKind.Bundler,
    target: 99, // ScriptTarget.ESNext,
    noEmit: true,
  };
  const libFiles = new LibFiles(monaco);
  const compilerOptionsChangeEmitter = new EventTrigger(new monaco.Emitter());
  const extraLibsChangeEmitter = new EventTrigger(new monaco.Emitter());

  const api = {
    addExtraLib: (
      content: string,
      _filePath?: string,
    ): monacoNS.IDisposable => {
      let filePath: string;
      if (typeof _filePath === "undefined") {
        filePath = `ts:extralib-${Math.random().toString(36).substring(2, 15)}`;
      } else {
        filePath = _filePath;
      }
      if (!libFiles.addExtraLib(content, filePath)) {
        // no-op, there already exists an extra lib with this content
        return { dispose: () => {} };
      }

      extraLibsChangeEmitter.fire();
      return {
        dispose: () => {
          if (libFiles.removeExtraLib(filePath)) {
            extraLibsChangeEmitter.fire();
          }
        },
      };
    },
  };
  monaco.languages["typescript"] = api;
  monaco.languages["javascript"] = api;

  const vfs = Reflect.get(monaco.editor, "vfs");
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
            libFiles.addExtraLib(dts, type);
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

  return {
    importMap,
    compilerOptions,
    libFiles,
    onCompilerOptionsChange: compilerOptionsChangeEmitter.event,
    onExtraLibsChange: extraLibsChangeEmitter.event,
  };
};

export const init = async (monaco: typeof monacoNS): Promise<SetupData> => {
  if (setupData) {
    return setupData;
  }
  return setupData = createSetupData(monaco);
};
