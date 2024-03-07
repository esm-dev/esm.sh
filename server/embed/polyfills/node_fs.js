// https://nodejs.org/api/fs.html

// copied from https://github.com/jspm/jspm-core/blob/main/src-browser/fs.js

import memfs from "/memfs@4.4.7";
const { vol, createFsFromVolume } = memfs;
import { Buffer } from "./node_buffer.js";
import { fileURLToPath } from "./node_url.js";

function unimplemented(name) {
  throw new Error(`Node.js fs ${name} is not supported in the browser`);
}

vol.fromNestedJSON({
  "/dev": { stdin: "", stdout: "", stderr: "" },
  "/usr/bin": {},
  "/home": {},
  "/tmp": {},
});

vol.releasedFds = [2, 1, 0];
vol.openSync("/dev/stdin", "w");
vol.openSync("/dev/stdout", "r");
vol.openSync("/dev/stderr", "r");
watchStdo("/dev/stdout", 1, console.log);
watchStdo("/dev/stderr", 2, console.error);
function watchStdo(path, fd, listener) {
  let oldSize = 0;
  const decoder = new TextDecoder();
  vol.watch(path, "utf8", () => {
    const { size } = vol.fstatSync(fd);
    const buf = Buffer.alloc(size - oldSize);
    vol.readSync(fd, buf, 0, buf.length, oldSize);
    oldSize = size;
    listener(decoder.decode(buf, { stream: true }));
  });
}

const fs = createFsFromVolume(vol);

fs.opendir = () => unimplemented("opendir");
fs.opendirSync = () => unimplemented("opendirSync");
fs.promises.opendir = () => unimplemented("promises.opendir");
fs.cp = () => unimplemented("cp");
fs.cpSync = () => unimplemented("cpSync");
fs.promises.cp = () => unimplemented("promises.cp");
fs.readv = () => unimplemented("readv");
fs.readvSync = () => unimplemented("readvSync");
fs.rm = () => unimplemented("rm");
fs.rmSync = () => unimplemented("rmSync");
fs.promises.rm = () => unimplemented("promises.rm");
fs.Dir = () => unimplemented("Dir");
fs.promises.watch = () => unimplemented("promises.watch");

fs.FileReadStream = fs.ReadStream;
fs.FileWriteStream = fs.WriteStream;

function handleFsUrl(url, isSync) {
  if (url.protocol === "file:") {
    return fileURLToPath(url);
  }
  if (url.protocol === "https:" || url.protocol === "http:") {
    const path = "\\\\url\\" + url.href.replaceAll(/\//g, "\\\\");
    if (existsSync(path)) {
      return path;
    }
    if (isSync) {
      throw new Error(
        `Cannot sync request URL ${url} via FS. JSPM FS support for network URLs requires using async FS methods or priming the MemFS cache first with an async request before a sync request.`,
      );
    }
    return (async () => {
      const res = await fetch(url);
      if (!res.ok) {
        throw new Error(`Unable to fetch ${url.href}, ${res.status}`);
      }
      const buf = await res.arrayBuffer();
      writeFileSync(path, Buffer.from(buf));
      return path;
    })();
  }
  throw new Error("URL " + url + " not supported in JSPM FS implementation.");
}

function wrapFsSync(fn) {
  return function (path, ...args) {
    if (path instanceof URL) {
      return fn(handleFsUrl(path, true), ...args);
    }
    return fn(path, ...args);
  };
}

function wrapFsPromise(fn) {
  return async function (path, ...args) {
    if (path instanceof URL) {
      return fn(await handleFsUrl(path), ...args);
    }
    return fn(path, ...args);
  };
}

function wrapFsCallback(fn) {
  return function (path, ...args) {
    const cb = args[args.length - 1];
    if (path instanceof URL && typeof cb === "function") {
      handleFsUrl(path).then((path) => {
        fn(path, ...args);
      }, cb);
    } else {
      fn(path, ...args);
    }
  };
}

fs.promises.readFile = wrapFsPromise(fs.promises.readFile);
fs.readFile = wrapFsCallback(fs.readFile);
fs.readFileSync = wrapFsSync(fs.readFileSync);

export const {
  appendFile,
  appendFileSync,
  access,
  accessSync,
  chown,
  chownSync,
  chmod,
  chmodSync,
  close,
  closeSync,
  copyFile,
  copyFileSync,
  cp,
  cpSync,
  createReadStream,
  createWriteStream,
  exists,
  existsSync,
  fchown,
  fchownSync,
  fchmod,
  fchmodSync,
  fdatasync,
  fdatasyncSync,
  fstat,
  fstatSync,
  fsync,
  fsyncSync,
  ftruncate,
  ftruncateSync,
  futimes,
  futimesSync,
  lchown,
  lchownSync,
  lchmod,
  lchmodSync,
  link,
  linkSync,
  lstat,
  lstatSync,
  mkdir,
  mkdirSync,
  mkdtemp,
  mkdtempSync,
  open,
  openSync,
  opendir,
  opendirSync,
  readdir,
  readdirSync,
  read,
  readSync,
  readv,
  readvSync,
  readFile,
  readFileSync,
  readlink,
  readlinkSync,
  realpath,
  realpathSync,
  rename,
  renameSync,
  rm,
  rmSync,
  rmdir,
  rmdirSync,
  stat,
  statSync,
  symlink,
  symlinkSync,
  truncate,
  truncateSync,
  unwatchFile,
  unlink,
  unlinkSync,
  utimes,
  utimesSync,
  watch,
  watchFile,
  writeFile,
  writeFileSync,
  write,
  writeSync,
  writev,
  writevSync,
  Dir,
  Dirent,
  Stats,
  ReadStream,
  WriteStream,
  FileReadStream,
  FileWriteStream,
  _toUnixTimestamp,
  constants: { F_OK, R_OK, W_OK, X_OK },
  constants,
  promises,
} = fs;

export default fs;
