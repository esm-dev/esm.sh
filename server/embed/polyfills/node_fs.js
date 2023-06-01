// https://nodejs.org/api/fs.html

function panic() {
  throw new Error(
    `[esm.sh] "node:fs" is not supported in browser environment.`,
  );
}

export let F_OK = null;
export let R_OK = null;
export let W_OK = null;
export let X_OK = null;

export function access() {
  panic();
}

export function accessSync() {
  panic();
}

export function appendFile() {
  panic();
}

export function appendFileSync() {
  panic();
}

export function chmod() {
  panic();
}

export function chmodSync() {
  panic();
}

export function chown() {
  panic();
}

export function chownSync() {
  panic();
}

export function close() {
  panic();
}

export function closeSync() {
  panic();
}

export let constants = new Proxy({}, { get: () => null });
export function copyFile() {
  panic();
}

export function copyFileSync() {
  panic();
}

export function createReadStream() {
  panic();
}

export function createWriteStream() {
  panic();
}

export function Dir() {
  panic();
}

export function Dirent() {
  panic();
}

export function exists() {
  panic();
}

export function existsSync() {
  panic();
}

export function fdatasync() {
  panic();
}

export function fdatasyncSync() {
  panic();
}

export function fstat() {
  panic();
}

export function fstatSync() {
  panic();
}

export function fsync() {
  panic();
}

export function fsyncSync() {
  panic();
}

export function ftruncate() {
  panic();
}

export function ftruncateSync() {
  panic();
}

export function futimes() {
  panic();
}

export function futimesSync() {
  panic();
}

export function link() {
  panic();
}

export function linkSync() {
  panic();
}

export function lstat() {
  panic();
}

export function lstatSync() {
  panic();
}

export function mkdir() {
  panic();
}

export function mkdirSync() {
  panic();
}

export function mkdtemp() {
  panic();
}

export function mkdtempSync() {
  panic();
}

export function open() {
  panic();
}

export function openSync() {
  panic();
}

export function read() {
  panic();
}

export function readSync() {
  panic();
}

export let promises = new Proxy({}, {
  get: (_t, prop) => _e(`promises/${prop}`),
});
export function readdir() {
  panic();
}

export function readdirSync() {
  panic();
}

export function readFile() {
  panic();
}

export function readFileSync() {
  panic();
}

export function readlink() {
  panic();
}

export function readlinkSync() {
  panic();
}

export function realpath() {
  panic();
}

export function realpathSync() {
  panic();
}

export function rename() {
  panic();
}

export function renameSync() {
  panic();
}

export function rmdir() {
  panic();
}

export function rmdirSync() {
  panic();
}

export function rm() {
  panic();
}

export function rmSync() {
  panic();
}

export function stat() {
  panic();
}

export function Stats() {
  panic();
}

export function statSync() {
  panic();
}

export function symlink() {
  panic();
}

export function symlinkSync() {
  panic();
}

export function truncate() {
  panic();
}

export function truncateSync() {
  panic();
}

export function unlink() {
  panic();
}

export function unlinkSync() {
  panic();
}

export function utimes() {
  panic();
}

export function utimesSync() {
  panic();
}

export function watch() {
  panic();
}

export function watchFile() {
  panic();
}

export function write() {
  panic();
}

export function writeSync() {
  panic();
}

export function writeFile() {
  panic();
}

export function writeFileSync() {
  panic();
}

export default {
  access,
  accessSync,
  appendFile,
  appendFileSync,
  chmod,
  chmodSync,
  chown,
  chownSync,
  close,
  closeSync,
  constants,
  copyFile,
  copyFileSync,
  createReadStream,
  createWriteStream,
  Dir,
  Dirent,
  exists,
  existsSync,
  F_OK,
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
  promises,
  R_OK,
  read,
  readdir,
  readdirSync,
  readFile,
  readFileSync,
  readlink,
  readlinkSync,
  readSync,
  realpath,
  realpathSync,
  rename,
  renameSync,
  rm,
  rmdir,
  rmdirSync,
  rmSync,
  stat,
  Stats,
  statSync,
  symlink,
  symlinkSync,
  truncate,
  truncateSync,
  unlink,
  unlinkSync,
  utimes,
  utimesSync,
  W_OK,
  watch,
  watchFile,
  write,
  writeFile,
  writeFileSync,
  writeSync,
  X_OK,
};
