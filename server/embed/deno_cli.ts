export type ImportMap = {
  readonly imports: Record<string, string>;
  readonly scopes: Record<string, Record<string, string>>;
};

export type Package = {
  readonly name: string;
  readonly version: string;
  readonly subModule?: string;
  readonly alias?: string;
  readonly dependencies?: Record<string, string>;
  readonly peerDependencies?: Record<string, string>;
  readonly exports?: Record<string, unknown> | string;
};

let VERSION = "v{VERSION}";
let importMapFile = "import_map.json";

async function add(args: string[], options: Record<string, string>) {
  if (options.alias && args.length > 1) {
    console.error(
      `%cerror%c: Cannot use --alias with multiple packages`,
      "color:red",
      "",
    );
    Deno.exit(1);
  }

  const importMap = await loadImportMap();
  const pkgs = (await Promise.all(args.map(fetchPkgInfo))).filter(Boolean);
  const alias = pkgs.length === 1 ? options.alias : undefined;
  if (alias) {
    (pkgs[0] as Record<string, unknown>).alias = alias;
  }

  await Promise.all(
    pkgs.map((pkg) => addPkgToImportMap(pkg!, importMap)),
  );
  await saveImportMap(importMap);
  console.log(
    `Added ${pkgs.length} packages to %c${
      importMapFile.split(/[\/\\]/g).pop()
    }`,
    "color:blue",
  );
  if (pkgs.length > 0) {
    console.log(
      pkgs.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        const { name, version, subModule } = pkg!;
        let msg = `${tab} ${name}@${version}`;
        if (subModule) {
          msg = `${msg}/${subModule}`;
        }
        if (alias) {
          msg = `${msg} (alias: ${alias})`;
        }
        return msg;
      }).join("\n"),
    );
  }
}

async function update(args: string[], options: Record<string, string>) {
  const importMap = await loadImportMap();
  const latest = "latest" in options;
  const toUpdate =
    (args.length === 0
      ? Object.keys(importMap.imports).filter((name) =>
        importMap.imports[name].startsWith("https://esm.sh/")
      ).map((name) => {
        let version: string;
        if (latest) {
          version = "latest";
        } else {
          const url = importMap.imports[name];
          const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
          if (!v.includes("-")) {
            version = v.split(".").slice(0, 2).join(".");
          } else {
            version = v;
          }
        }
        return `${name}@${version}`;
      })
      : args.filter((name) =>
        name in importMap.imports ||
        name.slice(0, name.lastIndexOf("@")) in importMap.imports
      ).map((name) => {
        let version: string;
        if ((name.startsWith("@") ? name.slice(1) : name).includes("@")) {
          const a = name.split("@");
          version = a.pop()!;
          name = a.join("@");
        } else if (latest) {
          version = "latest";
        } else {
          const url = importMap.imports[name] ??
            importMap.imports[name.slice(0, name.lastIndexOf("@"))];
          const [, v] = url.match(/@(\d+\.\d+\.\d+(-[a-z0-9\-\.]+)?)/)!;
          if (!v.includes("-")) {
            version = v.split(".").slice(0, 2).join(".");
          } else {
            version = v;
          }
        }
        return `${name}@${version}`;
      }));
  const pkgs = (await Promise.all(toUpdate.map(fetchPkgInfo))).filter(Boolean);
  const updates: Package[] = [];

  for (const pkg of pkgs) {
    if (await addPkgToImportMap(pkg!, importMap)) {
      updates.push(pkg!);
    }
  }

  await saveImportMap(importMap);
  console.log(`updates ${updates.length} packages`);
  if (updates.length > 0) {
    console.log(
      updates.map((pkg, index) => {
        const tab = index === pkgs.length - 1 ? "└─" : "├─";
        return `${tab} ${pkg.name}@${pkg.version}`;
      }).join("\n"),
    );
  }
}

async function remove(args: string[], options: Record<string, string>) {
  const importMap = await loadImportMap();
  const toRemove = args.filter((name) => name in importMap.imports);
  for (const name of toRemove) {
    Reflect.deleteProperty(importMap.imports, name);
    Reflect.deleteProperty(importMap.imports, name + "/");
    Reflect.deleteProperty(importMap.scopes, name);
    Reflect.deleteProperty(importMap.scopes, name + "/");
  }
  await saveImportMap(importMap);
  console.log(`Removed ${toRemove.length} packages`);
  if (toRemove.length > 0) {
    console.log(
      toRemove.map((name, index) => {
        const tab = index === toRemove.length - 1 ? "└─" : "├─";
        return `${tab} ${name}`;
      }).join("\n"),
    );
  }
}

async function init(args: string[], options: Record<string, string>) {
  const config = await getDenoConfig();
  const importMap = await loadImportMap();
  if (!isNEString(config.importMap)) {
    config.importMap = importMapFile;
  }
  const tasks = config.tasks as undefined | Record<string, string>;
  if (!tasks || !("npm:add" in tasks)) {
    config.tasks = {
      ...tasks,
      "npm:add": "deno run -A https://esm.sh add",
      "npm:remove": "deno run -A https://esm.sh remove",
      "npm:update": "deno run -A https://esm.sh update",
    };
  }
  await Deno.writeTextFile(
    "deno.json",
    JSON.stringify(config, null, 2),
  );
  await saveImportMap(importMap);
  console.log("Initialized %cdeno.json%c, 3 task added:", "color:green", "");
  console.log(
    "  - %cdeno task npm:add%c [packages...]",
    "color:blue",
    "color:gray",
  );
  console.log(
    "  - %cdeno task npm:remove%c [packages...]",
    "color:blue",
    "color:gray",
  );
  console.log(
    "  - %cdeno task npm:update%c [packages...]",
    "color:blue",
    "color:gray",
  );
}

const cache = new Map<string, Package>();
async function fetchPkgInfo(name: string): Promise<Package | null> {
  if (cache.has(name)) {
    return Promise.resolve(cache.get(name)!);
  }

  let pkgName: string;
  let subModule: string | undefined;
  const a = name.split("/");
  if (name.startsWith("@")) {
    if (a.length < 2) {
      return null;
    }
    pkgName = a[0] + "/" + a[1];
    subModule = a[2];
  } else {
    pkgName = a[0];
    if (a.length > 1) {
      subModule = a.slice(1).join("/");
    }
  }

  const res = await fetch(`https://esm.sh/${pkgName}/package.json`);
  if (res.status === 404) {
    console.error(`%cerror%c: Package "${pkgName}" not found`, "color:red", "");
    Deno.exit(1);
  }

  if (!res.ok) {
    console.error(`%cerror%c: Failed to fetch "${pkgName}"`, "color:red", "");
    Deno.exit(1);
  }

  const pkg = await res.json();
  if (!pkg.name || !pkg.version) {
    console.error(
      `%cerror%c: Invalid package.json of "${pkgName}"`,
      "color:red",
      "",
    );
    Deno.exit(1);
  }

  pkg.subModule = subModule;
  cache.set(name, pkg);
  return pkg;
}

async function loadImportMap(): Promise<ImportMap> {
  const importMap: ImportMap = { imports: {}, scopes: {} };
  try {
    const raw = (await Deno.readTextFile(importMapFile)).trim();
    if (raw.startsWith("{") && raw.endsWith("}")) {
      const { imports, scopes } = JSON.parse(raw);
      if (imports) {
        Object.assign(importMap.imports, imports);
      }
      if (scopes) {
        Object.assign(importMap.scopes, scopes);
      }
    }
  } catch (err) {
    if (!(err instanceof Deno.errors.NotFound)) {
      throw err;
    }
  }
  return importMap;
}

async function saveImportMap(importMap: ImportMap): Promise<void> {
  // clean up
  for (const importName in importMap.imports) {
    for (const [scopeName, scope] of Object.entries(importMap.scopes)) {
      if (importName in scope) {
        Reflect.deleteProperty(scope, importName);
        if (Object.keys(scope).length === 0) {
          Reflect.deleteProperty(importMap.scopes, scopeName);
        }
      }
    }
  }

  // sort
  const sortedImports = sortImports(importMap.imports);
  const sortedScopes = Object.fromEntries(
    Object.entries(importMap.scopes).sort(sortByKey).map((
      [key, scope],
    ) => [key, sortImports(scope)]),
  );

  // write
  await Deno.writeTextFile(
    importMapFile,
    JSON.stringify({ imports: sortedImports, scopes: sortedScopes }, null, 2),
  );
}

async function getDenoConfig(): Promise<Record<string, unknown>> {
  try {
    const config = await Deno.readTextFile("deno.json");
    return JSON.parse(config);
  } catch (err) {
    if (err instanceof Deno.errors.NotFound) {
      return {};
    }
    throw err;
  }
}

async function addPkgToImportMap(
  pkg: Package,
  importMap: ImportMap,
): Promise<boolean> {
  let [pkgUrl, withExports] = getPkgUrl(pkg);
  let aliasName = pkg.alias ?? pkg.name;
  if (pkg.subModule) {
    if (!pkg.alias) {
      aliasName += "/" + pkg.subModule;
    }
    pkgUrl += "/" + pkg.subModule;
  }
  if (importMap.imports[aliasName] === pkgUrl) {
    return false;
  }
  importMap.imports[aliasName] = pkgUrl;
  if (withExports && !pkg.subModule) {
    importMap.imports[aliasName + "/"] = pkgUrl + "/";
  }
  if (pkg.dependencies) {
    if (!pkg.subModule) {
      importMap.scopes[aliasName] = {};
    }
    if (withExports || pkg.subModule) {
      importMap.scopes[pkg.name + "/"] = {};
    }
    for (const [depName, depVersion] of Object.entries(pkg.dependencies)) {
      const dep = `${depName}@${depVersion}`;
      const depPkg = await fetchPkgInfo(dep);
      if (depPkg) {
        const depUrl =
          `https://esm.sh/${VERSION}/${depPkg.name}@${depPkg.version}`;
        if (!pkg.subModule) {
          importMap.scopes[aliasName][depName] = depUrl;
        }
        if (withExports || pkg.subModule) {
          importMap.scopes[pkg.name + "/"][depName] = depUrl;
        }
      }
    }
  }
  return true;
}

function getPkgUrl(pkg: Package): [url: string, withExports: boolean] {
  const { name, version, exports, dependencies, peerDependencies } = pkg;
  const withExports = typeof exports === "object" &&
    Object.keys(exports).some((key) =>
      key.length >= 3 && key.startsWith("./") && key !== "./package.json"
    );

  if (
    (dependencies && Object.keys(dependencies).length > 0) ||
    (peerDependencies && Object.keys(peerDependencies).length > 0)
  ) {
    return [`https://esm.sh/${VERSION}/*${name}@${version}`, withExports];
  }
  return [`https://esm.sh/${VERSION}/${name}@${version}`, withExports];
}

function sortImports(imports: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(imports).sort(sortByValue),
  );
}

function sortByKey(a: [string, unknown], b: [string, unknown]) {
  const [aName] = a;
  const [bName] = b;
  if (aName < bName) {
    return -1;
  }
  if (aName > bName) {
    return 1;
  }
  return 0;
}

function sortByValue(a: [string, string], b: [string, string]) {
  const aValue = a[1].replace("/*", "/");
  const bValue = b[1].replace("/*", "/");
  if (aValue < bValue) {
    return -1;
  }
  if (aValue > bValue) {
    return 1;
  }
  return 0;
}

function isNEString(a: unknown): a is string {
  return typeof a === "string" && a !== "";
}

function parseFlags(
  raw: string[],
): [args: string[], options: Record<string, string>] {
  const args: string[] = [];
  const options: Record<string, string> = {};
  let argCur: string | null = null;
  for (const arg of raw) {
    if (arg.startsWith("--")) {
      if (argCur) {
        options[argCur] = "";
        argCur = null;
      }
      if (arg.includes("=")) {
        const [name, value] = arg.slice(2).split("=");
        options[name] = value;
      } else {
        argCur = arg.slice(2);
      }
    } else if (argCur) {
      options[argCur] = arg;
      argCur = null;
    } else {
      args.push(arg);
    }
  }
  if (argCur) {
    options[argCur] = "";
    argCur = null;
  }
  return [args, options];
}

if (import.meta.main) {
  const start = performance.now();
  const [command, ...args] = Deno.args;
  const commands = {
    add,
    remove,
    update,
    init,
  };

  if (command === undefined || !(command in commands)) {
    console.error(`Command "${command}" not found`);
    Deno.exit(1);
  }

  try {
    const config = await getDenoConfig();
    if (isNEString(config.importMap)) {
      importMapFile = config.importMap;
    }
    await commands[command as keyof typeof commands](...parseFlags(args));
    console.log(`✨ Done in ${(performance.now() - start).toFixed(2)}ms`);
  } catch (error) {
    throw error;
  }
}
