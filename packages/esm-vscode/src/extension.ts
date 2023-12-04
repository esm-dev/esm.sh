import * as vscode from "vscode";
import type { ImportMap } from "./typescript-esm-plugin.ts";
import { getImportMapFromHtml } from "./util.ts";

interface ProjectConfig {
  importMap?: ImportMap;
}

interface TSApi {
  updateConfig: (config: ProjectConfig) => void;
}

export async function activate(context: vscode.ExtensionContext) {
  const { commands, workspace, window } = vscode;

  let tsApi: TSApi;
  try {
    tsApi = await setupTSLsp();
    console.log("vscode typescript extension activated.");
  } catch (e) {
    console.error(e);
  }

  context.subscriptions.push(
    workspace.onDidSaveTextDocument(async (document) => {
      const name = workspace.asRelativePath(document.uri);
      if (name === "index.html") {
        const importMap = getImportMapFromHtml(document.getText());
        tsApi.updateConfig({ importMap });
      }
    }),
  );

  context.subscriptions.push(
    commands.registerCommand(
      "esmsh.rebuildImportMap",
      () => {
        window.showInformationMessage("import map rebuilt");
      },
    ),
  );
}

async function setupTSLsp() {
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
      const old = JSON.stringify(config);
      Object.assign(config, c);
      if (old !== JSON.stringify(config)) {
        api.configurePlugin("typescript-esm-plugin", config);
      }
    },
  };
}

export function deactivate() {}
