import type * as monacoNS from "monaco-editor-core";

export interface ExtraLib {
  content: string;
  version: number;
}

export interface SetupData {
  libFiles: LibFiles;
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

// javascript and typescript share the setup data
let setupData: SetupData | null = null;

export const init = (
  monaco: typeof monacoNS,
): SetupData => {
  if (setupData) {
    return setupData;
  }

  const libFiles = new LibFiles(monaco);
  const extraLibsChangeEmitter = new monaco.Emitter<void>();

  let extraLibsChanged = false;
  const fireExtraLibsChange = () => {
    if (!extraLibsChanged) {
      extraLibsChanged = true;
      queueMicrotask(() => {
        extraLibsChanged = false;
        extraLibsChangeEmitter.fire();
      });
    }
  };

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

      fireExtraLibsChange();
      return {
        dispose: () => {
          if (libFiles.removeExtraLib(filePath)) {
            fireExtraLibsChange();
          }
        },
      };
    },
  };
  monaco.languages["typescript"] = api;
  monaco.languages["javascript"] = api;

  setupData = {
    libFiles,
    onExtraLibsChange: extraLibsChangeEmitter.event,
  };
  return setupData;
};
