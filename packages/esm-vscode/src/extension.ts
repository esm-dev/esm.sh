import * as vscode from "vscode";
import type { ImportMap } from "./typescript-esmsh-plugin.ts";
import {
  debunce,
  getImportMapFromHtml,
  insertImportMap,
  regexpNpmNaming,
  sortByVersion,
} from "./util.ts";

interface ProjectConfig {
  importMap?: ImportMap;
}

interface TSApi {
  updateConfig: (config: ProjectConfig) => void;
}

const jsxRuntimes = [
  "react",
  "preact",
  "solid-js",
];

export async function activate(context: vscode.ExtensionContext) {
  const { commands, workspace, window } = vscode;

  let tsApi: TSApi;
  try {
    tsApi = await ensureTsApi();
  } catch (e) {
    console.error(e);
  }

  const onIndexHtmlChange = debunce((html: string) => {
    const importMap = getImportMapFromHtml(html);
    tsApi.updateConfig({ importMap });
  }, 500);

  context.subscriptions.push(
    workspace.onDidSaveTextDocument((document) => {
      const name = workspace.asRelativePath(document.uri);
      if (name === "index.html") {
        onIndexHtmlChange(document.getText());
      }
    }),
  );

  context.subscriptions.push(
    commands.registerCommand(
      "esmsh.addModule",
      async () => {
        const name = await window.showInputBox({
          placeHolder: "Enter module name, e.g. lodash",
          validateInput: (name: string) => {
            if (!name.trim()) {
              return null;
            }
            let scope = "";
            let pkgName = name;
            if (name.startsWith("@")) {
              const parts = name.split("/");
              if (parts.length < 2) {
                return "Invalid Module Name";
              }
              scope = parts[0] + "/";
              pkgName = parts[1];
            }
            pkgName = pkgName.split("@")[0];
            if (
              (scope && !regexpNpmNaming.test(scope)) ||
              !regexpNpmNaming.test(pkgName)
            ) {
              return "Invalid Module Name";
            }
            return null;
          },
        });
        if (!name) {
          return;
        }

        // TODO: support multiple packages
        let pkgName = name.trim().split(" ")[0];
        let scope = "";
        let version = "";
        if (pkgName.startsWith("@")) {
          const parts = pkgName.split("/");
          scope = parts[0];
          pkgName = parts[1];
        }
        if (pkgName.includes("@")) {
          const parts = pkgName.split("@");
          pkgName = parts[0];
          version = parts[1];
        }
        if (scope) {
          pkgName = scope + "/" + pkgName;
        }
        window.withProgress({
          location: vscode.ProgressLocation.Window,
          cancellable: true,
          title: `Searching '${pkgName}'...`,
        }, async () => {
          try {
            const res = await fetch(`https://registry.npmjs.org/${pkgName}`);
            if (!res.ok) {
              window.showErrorMessage(`Could not find '${pkgName}'`);
              return;
            }

            const pkgInfo = await res.json();
            const distTags = Object.keys(pkgInfo["dist-tags"]).map((dist) => ({
              label: pkgName,
              description: `<${dist}> ${pkgInfo["dist-tags"][dist]}`,
            }));
            const allVersions = Object.keys(pkgInfo.versions).sort(
              sortByVersion,
            ).map((version) => ({ label: pkgName, description: version }));
            const versions = version
              ? allVersions.filter((v) => v.description.startsWith(version))
              : distTags.concat(allVersions);
            if (versions.length === 0) {
              window.showErrorMessage(`Could not find '${pkgName}@${version}'`);
              return;
            }
            window.showQuickPick(
              versions,
              {
                placeHolder: `Select a version of '${pkgName}':`,
                matchOnDescription: true,
              },
            ).then(async (item) => {
              if (!item) {
                return;
              }

              const uris = await workspace.findFiles("index.html");
              if (uris.length === 0) {
                window.showErrorMessage("No index.html found");
                return;
              }

              const uri = uris[0];
              const document = await workspace.openTextDocument(uri);
              const importMap = getImportMapFromHtml(document.getText());
              const pkgName = item.label;
              const version = item.description.split(" ")[1] ??
                item.description;
              const imports = importMap.imports || (importMap.imports = {});
              if (jsxRuntimes.includes(pkgName)) {
                imports["@jsxImportSource"] =
                  `https://esm.sh/${pkgName}@${version}`;
              }
              imports[pkgName] = `https://esm.sh/${pkgName}@${version}`;
              imports[pkgName + "/"] = `https://esm.sh/${pkgName}@${version}/`;
              const newHtml = new TextEncoder().encode(
                insertImportMap(document.getText(), importMap),
              );
              workspace.fs.writeFile(uri, newHtml);
              tsApi.updateConfig({ importMap });
            });
          } catch (error) {
            window.showErrorMessage(`Could not find '${name}'`);
          }
        });
      },
    ),
  );
}

async function ensureTsApi() {
  const tsExtension = vscode.extensions.getExtension(
    "vscode.typescript-language-features",
  );
  if (!tsExtension) {
    throw new Error("vscode.typescript-language-features not found");
  }
  await tsExtension.activate();
  const api = tsExtension.exports.getAPI(0);
  if (!api) {
    throw new Error("vscode.typescript-language-features api not found");
  }
  const config: ProjectConfig = {};
  return {
    updateConfig: (c: ProjectConfig) => {
      const old = JSON.stringify(config);
      Object.assign(config, c);
      if (old !== JSON.stringify(config)) {
        api.configurePlugin("typescript-esmsh-plugin", config);
      }
    },
  };
}

export function deactivate() {}
