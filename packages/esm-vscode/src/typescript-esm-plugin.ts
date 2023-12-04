import TS from "typescript/lib/tsserverlibrary";
import { createHash } from "node:crypto";
import { homedir } from "node:os";
import { dirname, join } from "node:path";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { getImportMapFromHtml } from "./util.ts";

export interface ImportMap {
  imports?: Record<string, string>;
  scopes?: Record<string, Record<string, string>>;
}

export interface PreprocessedImportMap {
  jsxImportSource?: string;
  alias?: Record<string, string>;
  trailingSlash?: [string, string][];
}

export interface ProjectConfig {
  importMap: PreprocessedImportMap;
}

class Plugin implements TS.server.PluginModule {
  #typescript: typeof TS;
  #projectConfig: ProjectConfig = { importMap: {} };
  #declMap = new Map<string, Promise<void> | string>();
  #logger: { i: number; info(s: string, ...args: any[]): void };
  #refresh = () => {};

  static userAgent =
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36";

  constructor(ts: typeof TS) {
    this.#typescript = ts;
  }

  create(info: TS.server.PluginCreateInfo): TS.LanguageService {
    const { languageService, languageServiceHost, project } = info;
    const cwd = project.getCurrentDirectory();
    const home = homedir();
    const esmCacheDir = join(home, ".cache/esm.sh");
    const esmCacheMetaDir = join(esmCacheDir, "meta");
    if (!existsSync(esmCacheMetaDir)) {
      mkdirSync(esmCacheMetaDir, { recursive: true });
    }

    // @ts-ignore
    this.#logger = DEBUG
      ? {
        i: 0,
        info(s: string, ...args: any[]) {
          const filename = join(cwd, "typescript-esm-plugin.log");
          if (this.i === 0) {
            this.i = 1;
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
          writeFileSync(filename, lines.join("\n") + "\n\n", {
            encoding: "utf8",
            flag: "a+",
            mode: 0o666,
          });
        },
      }
      : { i: 0, info() {} };

    // refresh diagnostics when config changed
    this.#refresh = () => {
      project.refreshDiagnostics();
      project.updateGraph();
      languageService.getProgram()?.emit();
    };

    // load import map from index.html if exists
    const indexHtml = join(cwd, "index.html");
    try {
      if (existsSync(indexHtml)) {
        const html = readFileSync(indexHtml, "utf-8");
        const importMap = getImportMapFromHtml(html);
        this.#logger.info("importmap set in index.html", importMap);
        this.#preprocessImportMap(importMap);
        this.#logger.info("importmap", this.#projectConfig.importMap);
      }
    } catch (error) {
      // ignore
    }

    // rewrite TS compiler options
    const tsGetCompilationSettings = languageServiceHost
      .getCompilationSettings.bind(languageServiceHost);
    languageServiceHost.getCompilationSettings = () => {
      const settings: TS.CompilerOptions = tsGetCompilationSettings();
      const jsxImportSource = this.#projectConfig.importMap?.jsxImportSource;
      if (jsxImportSource) {
        settings.jsx = TS.JsxEmit.ReactJSX;
        settings.jsxImportSource = jsxImportSource;
      }
      return settings;
    };

    // todo
    const tsResolveTypeReferenceDirectiveReferences = languageServiceHost
      .resolveTypeReferenceDirectiveReferences?.bind(languageServiceHost);
    if (tsResolveTypeReferenceDirectiveReferences) {
      languageServiceHost.resolveTypeReferenceDirectiveReferences = (
        typeDirectiveReferences: readonly (TS.FileReference | string)[],
        containingFile: string,
        ...rest
      ) => {
        this.#logger.info(
          "resolveTypeReferenceDirectiveReferences",
          typeDirectiveReferences,
        );
        return tsResolveTypeReferenceDirectiveReferences(
          typeDirectiveReferences,
          containingFile,
          ...rest,
        );
      };
    }

    // rewrite TS module resolution
    const tsResolveModuleNameLiterals = languageServiceHost
      .resolveModuleNameLiterals?.bind(languageServiceHost);
    if (tsResolveModuleNameLiterals) {
      const resolvedModule = (resolvedFileName: string, extension: string) => ({
        resolvedModule: {
          resolvedFileName,
          extension,
          resolvedUsingTsExtension: extension === ".d.ts",
        },
      });
      languageServiceHost.resolveModuleNameLiterals = (
        literals,
        containingFile: string,
        ...rest
      ) => {
        const resolvedModules = tsResolveModuleNameLiterals(
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
          this.#logger.info(
            "resolveModuleNameLiterals[" + i + "]: " + literals[i].text,
            { containingFile },
          );
          const literal = literals[i].text;
          const specifier = this.#applyImportMap(literal);
          if (isHttpUrlLike(specifier)) {
            const url = new URL(specifier);
            if (url.hostname === "esm.sh") {
              const isDts = specifier.endsWith(".d.ts");
              if (this.#declMap.has(specifier)) {
                const decl = this.#declMap.get(specifier);
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
                    fetch(specifier).then((res) => {
                      if (!res.ok) {
                        res.body?.cancel();
                        return Promise.reject(res.statusText);
                      }
                      return res.text();
                    }).then((dts) => {
                      const dtsDir = dirname(dtsFile);
                      if (!existsSync(dtsDir)) {
                        mkdirSync(dtsDir, { recursive: true });
                      }
                      writeFileSync(dtsFile, dts, "utf-8");
                      this.#declMap.set(specifier, "");
                      this.#refresh();
                      this.#logger.info("dts loaded: " + specifier);
                    });

                  this.#declMap.set(specifier, load().catch(cleanup));
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
                      headers: { "user-agent": Plugin.userAgent },
                    }).then((res) => {
                      res.body?.cancel();
                      if (!res.ok) {
                        return Promise.reject(res.statusText);
                      }
                      return res.headers.get("x-typescript-types") ?? "";
                    }).catch(() => {
                      cleanup();
                      return null;
                    });
                    if (dtsUrl) {
                      this.#logger.info(
                        "dts found: " + dtsUrl,
                      );
                      fetch(dtsUrl).then((res) => {
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
                        this.#logger.info(
                          "dts loaded: " + specifier + " -> " + url.href,
                        );
                      }).catch(cleanup);
                    } else if (dtsUrl === "") {
                      writeFileSync(metaFile, "{}", "utf-8");
                      this.#declMap.set(specifier, "");
                      this.#refresh();
                    }
                  };
                  this.#declMap.set(specifier, load());
                }
              }
            }
            return resolvedModule(specifier, ".js");
          }
          return { resolvedModule: undefined };
        });
      };
    }

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
    const alias = importMap.alias?.[specifier];
    if (alias) {
      return alias;
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
          (importMap.alias ?? (importMap.alias = {}))[k] = v;
        }
      }
    }
    // TODO: scopes
    this.#projectConfig.importMap = importMap;
  }
}

function isNEString(s: any): s is string {
  return typeof s === "string" && s.length > 0;
}

function isHttpUrlLike(name: string) {
  return name.startsWith("https://") || name.startsWith("http://");
}

export function init({ typescript }: { typescript: typeof TS }) {
  return new Plugin(typescript);
}
