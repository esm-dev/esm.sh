import { getMimeType } from "./mime.mjs";

/**
 * openFile for all runtimes.
 * @type {() => Promise<import("../types").FsFile>}
 */
let openFile;
if (typeof Deno !== "undefined") {
  openFile = async (path) => {
    try {
      const file = await Deno.open(path);
      const stat = await file.stat();
      return {
        size: stat.size,
        lastModified: stat.mtime?.getTime() ?? null,
        contentType: getMimeType(path),
        body: file.readable,
        close: () => Promise.resolve(file.close()),
      };
    } catch (error) {
      if (error instanceof Deno.errors.NotFound) {
        return null;
      }
      throw error;
    }
  };
} else if (typeof Bun !== "undefined") {
  openFile = async (path) => {
    const file = Bun.file(path);
    const found = await file.exists();
    if (!found) {
      return null;
    }
    return {
      size: file.size,
      lastModified: file.lastModified,
      contentType: getMimeType(path),
      body: await file.stream(),
      close: () => Promise.resolve(undefined),
    };
  };
} else if (typeof process === "object") {
  const fsPromise = import("node:fs/promises");
  openFile = async (path) => {
    const fs = await fsPromise;
    try {
      const file = await fs.open(path, "r");
      const stat = await file.stat();
      return {
        size: stat.size,
        lastModified: stat.mtime.getTime(),
        body: file.readableWebStream(),
        contentType: getMimeType(path),
        close: () => file.close(),
      };
    } catch (error) {
      if (error.code === "ENOENT") {
        return null;
      }
      throw error;
    }
  };
} else {
  throw new Error("no fs implementation");
}

export { openFile };
