import * as vscode from "vscode";
import type { ProjectConfig } from "./typescript-esm-plugin.ts";
import { getImportMap } from "./util.ts";

interface TSApi {
  updateConfig: (config: ProjectConfig) => void;
}

export async function activate(context: vscode.ExtensionContext) {
  const { commands, workspace, window } = vscode;

  let tsApi: TSApi;
  try {
    tsApi = await setupTSLang();
    console.log("vscode typescript extension activated.");
  } catch (e) {
    console.error(e);
  }

  // check the jsxImportSource in index.html
  const checkJSXImportSourceInIndexHtml = async (
    document: vscode.TextDocument,
  ) => {
    const importMap = await getImportMap(document);
    const jsxImportSource = importMap?.imports["@jsxImportSource"];
    if (jsxImportSource) {
      console.log("@jsxImportSource", jsxImportSource);
    }
    tsApi.updateConfig({ jsxImportSource });
  };
  workspace.findFiles("index.html").then((uris) => {
    uris.forEach((uri) => {
      workspace.openTextDocument(uri).then(checkJSXImportSourceInIndexHtml);
    });
  });
  context.subscriptions.push(workspace.onDidSaveTextDocument((document) => {
    const name = workspace.asRelativePath(document.uri);
    if (name === "index.html") {
      checkJSXImportSourceInIndexHtml(document);
    }
  }));

  context.subscriptions.push(
    commands.registerCommand(
      "esmsh.rebuildImportMap",
      () => {
        window.showInformationMessage("import map rebuilt");
      },
    ),
  );

  context.subscriptions.push(
    workspace.registerTextDocumentContentProvider(
      "esmsh",
      new class implements vscode.TextDocumentContentProvider {
        provideTextDocumentContent(uri: vscode.Uri): string {
          console.log("@@ provideTextDocumentContent", uri);
          return `/** useState hook */
        export function useState(a) {
          return [a,()=>{}]
        }
        `;
        }
      }(),
    ),
  );
}

async function setupTSLang() {
  const tsExtension = vscode.extensions.getExtension(
    "vscode.typescript-language-features",
  );
  if (!tsExtension) {
    throw new Error("vscode.typescript-language-features not found");
  }
  await tsExtension.activate();
  const api = tsExtension.exports.getAPI(0);
  const config: ProjectConfig = {};
  return {
    updateConfig: (c: ProjectConfig) => {
      Object.assign(config, c);
      api.configurePlugin("typescript-esm-plugin", config);
    },
  };
}

function debunce(fn: (...args: any[]) => void, delay: number) {
  let timer: NodeJS.Timeout;
  return (...args: any[]) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), delay);
  };
}

export function deactivate() {}
