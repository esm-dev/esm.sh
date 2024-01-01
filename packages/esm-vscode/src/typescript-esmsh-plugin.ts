import type TS from "typescript/lib/tsserverlibrary";
import { createHash } from "node:crypto";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { getImportMapFromHtml, isNEString, isValidEsmshUrl } from "./util.ts";

export interface ImportMap {
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

export interface PreprocessedImportMap {
  jsxImportSource?: string;
  imports?: Record<string, string>;
  trailingSlash?: [string, string][];
}

export interface ProjectConfig {
  importMap: PreprocessedImportMap;
}

class Plugin implements TS.server.PluginModule {
  #typescript: typeof TS;
  #projectConfig: ProjectConfig = { importMap: {} };
  #declMap = new Map<string, Promise<void> | string | 404>();
  #logger: { info(s: string, ...args: any[]): void } = { info() {} };
  #refresh = () => {};

  constructor(ts: typeof TS) {
    this.#typescript = ts;
  }

  create(info: TS.server.PluginCreateInfo): TS.LanguageService {
    const { languageService, languageServiceHost, project } = info;
    const home = homedir();
    const cwd = project.getCurrentDirectory();
    const esmCacheDir = join(home, ".cache/esm.sh");
    const esmCacheMetaDir = join(esmCacheDir, "meta");

    // ensure cache dir exists
    if (!existsSync(esmCacheMetaDir)) {
      mkdirSync(esmCacheMetaDir, { recursive: true });
    }

    // @ts-ignore
    this.#logger = DEBUG
      ? {
        info(s: string, ...args: any[]) {
          const filename = join(cwd, "typescript-esmsh-plugin.log");
          if (!this._reset) {
            this._reset = true;
            writeFileSync(filename, "", {
              encoding: "utf8",
              flag: "w",
              mode: 0o666,
            });
          }
          const lines = [s];
          if (args.length) {
            lines.push("---");
            lines.push(...args.map((arg) => JSON.stringify(arg, undefined, 2)));
            lines.push("---");
          }
          writeFileSync(filename, lines.join("\n") + "\n", {
            encoding: "utf8",
            flag: "a+",
            mode: 0o666,
          });
        },
      }
      : { info() {} };

    // reload projects and refresh diagnostics
    this.#refresh = () => {
      project.projectService.reloadProjects();
      project.refreshDiagnostics();
    };

    // load import map from index.html if exists
    try {
      const indexHtml = join(cwd, "index.html");
      if (existsSync(indexHtml)) {
        const html = readFileSync(indexHtml, "utf-8");
        const importMap = getImportMapFromHtml(html);
        this.#preprocessImportMap(importMap);
        this.#logger.info("load importmap from index.html", importMap);
      }
    } catch (error) {
      // ignore
    }

    // rewrite TS compiler options
    const getCompilationSettings = languageServiceHost
      .getCompilationSettings.bind(languageServiceHost);
    languageServiceHost.getCompilationSettings = () => {
      const settings: TS.CompilerOptions = getCompilationSettings();
      const jsxImportSource = this.#projectConfig?.importMap.jsxImportSource;
      if (jsxImportSource && !settings.jsxImportSource) {
        settings.jsx = this.#typescript.JsxEmit.ReactJSX;
        settings.jsxImportSource = jsxImportSource;
      }
      settings.allowImportingTsExtensions = true;
      return settings;
    };

    // rewrite TS module resolution
    const resolveModuleNameLiterals = languageServiceHost
      .resolveModuleNameLiterals?.bind(languageServiceHost);
    if (resolveModuleNameLiterals) {
      const resolvedModule = (resolvedFileName: string, extension: string) => {
        const resolvedUsingTsExtension = extension === ".d.ts";
        return {
          resolvedModule: {
            resolvedFileName,
            extension,
            resolvedUsingTsExtension,
          },
        };
      };
      languageServiceHost.resolveModuleNameLiterals = (
        literals,
        containingFile: string,
        ...rest
      ) => {
        const resolvedModules = resolveModuleNameLiterals(
          literals,
          containingFile,
          ...rest,
        );
        return resolvedModules.map((
          res: TS.ResolvedModuleWithFailedLookupLocations,
          i: number,
        ): typeof res => {
          if (res.resolvedModule) {
            return res;
          }
          let literal = literals[i].text;
          // fix relative path
          if (literal.startsWith("./") || literal.startsWith("../")) {
            const idx = containingFile.indexOf("/esm.sh/");
            if (idx) {
              literal = new URL(literal, "https:/" + containingFile.slice(idx)).href;
            }
          }
          const specifier = this.#applyImportMap(literal);
          const mod = isValidEsmshUrl(specifier);
          if (mod) {
            const { url } = mod;
            const isDts = specifier.endsWith(".d.ts");
            if (this.#declMap.has(specifier)) {
              const decl = this.#declMap.get(specifier);
              if (decl === 404) {
                return { resolvedModule: undefined };
              }
              if (typeof decl === "string") {
                if (decl) {
                  return resolvedModule(join(esmCacheDir, decl), ".d.ts");
                }
                if (isDts) {
                  return resolvedModule(
                    join(esmCacheDir, url.pathname),
                    ".d.ts",
                  );
                }
              }
              return resolvedModule(specifier, ".js");
            }

            this.#logger.info("missing module types declare: " + specifier);
            if (isDts) {
              const dtsFile = join(esmCacheDir, url.pathname);
              if (existsSync(dtsFile)) {
                this.#declMap.set(specifier, "");
                return resolvedModule(dtsFile, ".d.ts");
              }
              if (!this.#declMap.has(specifier)) {
                const cleanup = () => {
                  this.#declMap.delete(specifier);
                };
                const load = () =>
                  fetch(
                    specifier,
                    { headers: { "user-agent": "Deno/1.38.0" } },
                  ).then<string | 404>((res) => {
                    if (!res.ok) {
                      res.body?.cancel();
                      if (res.status === 404) {
                        return Promise.resolve(404 as const);
                      }
                      return Promise.reject(res.statusText);
                    }
                    return res.text();
                  }).then((dts) => {
                    if (dts === 404) {
                      this.#declMap.set(specifier, 404);
                    } else {
                      const dtsDir = dirname(dtsFile);
                      if (!existsSync(dtsDir)) {
                        mkdirSync(dtsDir, { recursive: true });
                      }
                      writeFileSync(dtsFile, dts, "utf-8");
                      this.#declMap.set(specifier, "");
                    }
                    this.#refresh();
                  });
                this.#declMap.set(specifier, load().catch(cleanup));
                return resolvedModule(specifier, ".js");
              }
            } else {
              const urlHash = createHash("sha256").update(specifier)
                .digest("hex");
              const metaFile = join(esmCacheMetaDir, urlHash);
              if (existsSync(metaFile)) {
                const meta = JSON.parse(readFileSync(metaFile, "utf-8"));
                if (meta.dts) {
                  this.#declMap.set(specifier, meta.dts);
                  return resolvedModule(join(esmCacheDir, meta.dts), ".d.ts");
                }
                this.#declMap.set(specifier, "");
                return resolvedModule(specifier, ".js");
              } else if (!this.#declMap.has(specifier)) {
                const cleanup = () => {
                  this.#declMap.delete(specifier);
                };
                const load = async () => {
                  const dtsUrl = await fetch(specifier, {
                    method: "HEAD",
                    headers: {
                      "user-agent":
                        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
                    },
                  }).then((res) => {
                    res.body?.cancel();
                    if (!res.ok) {
                      if (res.status === 404) {
                        return 404 as const;
                      }
                      return Promise.reject(res.statusText);
                    }
                    return res.headers.get("x-typescript-types") ?? "";
                  }).catch(() => {
                    cleanup();
                    return null;
                  });
                  if (dtsUrl === 404) {
                    this.#declMap.set(specifier, 404);
                    this.#refresh();
                  } else if (dtsUrl === "") {
                    writeFileSync(metaFile, "{}", "utf-8");
                    this.#declMap.set(specifier, "");
                    this.#refresh();
                  } else if (dtsUrl) {
                    fetch(
                      dtsUrl,
                      { headers: { "user-agent": "Deno/1.38.0" } },
                    ).then((res) => {
                      if (!res.ok) {
                        res.body?.cancel();
                        return Promise.reject(res.statusText);
                      }
                      return res.text().then((dts) => [res.url, dts]);
                    }).then(([resUrl, dts]) => {
                      const url = new URL(resUrl);
                      const dtsFile = join(esmCacheDir, url.pathname);
                      const dtsDir = dirname(dtsFile);
                      const meta = JSON.stringify({
                        dts: url.pathname,
                      });
                      if (!existsSync(dtsDir)) {
                        mkdirSync(dtsDir, { recursive: true });
                      }
                      writeFileSync(dtsFile, dts, "utf-8");
                      writeFileSync(metaFile, meta, "utf-8");
                      this.#declMap.set(specifier, url.pathname);
                      this.#refresh();
                    }).catch(cleanup);
                  }
                };
                this.#declMap.set(specifier, load());
                return resolvedModule(specifier, ".js");
              }
            }
          }
          return { resolvedModule: undefined };
        });
      };
    }

    // filter invalid auto imports
    const getCompletionsAtPosition = languageService.getCompletionsAtPosition;
    languageService.getCompletionsAtPosition = (
      fileName,
      position,
      options,
    ) => {
      const result = getCompletionsAtPosition(fileName, position, options);
      if (result) {
        result.entries = result.entries.filter((entry) => {
          return !entry.source?.includes("../.cache/esm.sh/");
        });
      }
      return result;
    };

    this.#logger.info("plugin created, typescrpt v" + this.#typescript.version);

    return languageService;
  }

  onConfigurationChanged(config: any): void {
    this.#logger.info("onConfigurationChanged", config);
    this.#preprocessImportMap(config.importMap);
    this.#refresh();
  }

  #applyImportMap(specifier: string) {
    const { importMap } = this.#projectConfig;
    const res = importMap.imports?.[specifier];
    if (res) {
      return res;
    }
    if (importMap.trailingSlash?.length) {
      for (const [prefix, replacement] of importMap.trailingSlash) {
        if (specifier.startsWith(prefix)) {
          return replacement + specifier.slice(prefix.length);
        }
      }
    }
    return specifier;
  }

  #preprocessImportMap(raw: ImportMap) {
    const importMap: PreprocessedImportMap = {};
    if (raw.imports) {
      for (const [k, v] of Object.entries(raw.imports)) {
        if (!k || !isNEString(v)) {
          continue;
        }
        if (v.endsWith("/")) {
          (importMap.trailingSlash ?? (importMap.trailingSlash = []))
            .push([k, v]);
        } else {
          if (k === "@jsxImportSource") {
            importMap.jsxImportSource = v;
          }
          (importMap.imports ?? (importMap.imports = {}))[k] = v;
        }
      }
    }
    // sort trailingSlash by prefix length
    importMap.trailingSlash?.sort((a, b) => b[0].split("/").length - a[0].split("/").length);
    // TODO: scopes
    this.#projectConfig.importMap = importMap;
  }
}

export function init({ typescript }: { typescript: typeof TS }) {
  return new Plugin(typescript);
}
