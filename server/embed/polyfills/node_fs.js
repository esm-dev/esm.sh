// https://nodejs.org/api/fs.html

function _e(name) {
  throw new Error(`[esm.sh] fs: '${name}' is not implemented`)
}

export let F_OK = null;
export let R_OK = null;
export let W_OK = null;
export let X_OK = null;

export let access = () => _e("accessaccess");
export let accessSync = () => _e("accessSyncaccessSync");
export let appendFile = () => _e("appendFile");
export let appendFileSync = () => _e("appendFileSync");
export let chmod = () => _e("chmod");
export let chmodSync = () => _e("chmodSync");
export let chown = () => _e("chown");
export let chownSync = () => _e("chownSync");
export let close = () => _e("close");
export let closeSync = () => _e("closeSync");
export let constants = new Proxy({}, { get: () => null });
export let copyFile = () => _e("copyFile");
export let copyFileSync = () => _e("copyFileSync");
export let Dir = () => _e("Dir");
export let Dirent = () => _e("Dirent");
export let exists = () => _e("exists");
export let existsSync = () => _e("existsSync");
export let fdatasync = () => _e("fdatasync");
export let fdatasyncSync = () => _e("fdatasyncSync");
export let fstat = () => _e("fstat");
export let fstatSync = () => _e("fstatSync");
export let fsync = () => _e("fsync");
export let fsyncSync = () => _e("fsyncSync");
export let ftruncate = () => _e("ftruncate");
export let ftruncateSync = () => _e("ftruncateSync");
export let futimes = () => _e("futimes");
export let futimesSync = () => _e("futimesSync");
export let link = () => _e("link");
export let linkSync = () => _e("linkSync");
export let lstat = () => _e("lstat");
export let lstatSync = () => _e("lstatSync");
export let mkdir = () => _e("mkdir");
export let mkdirSync = () => _e("mkdirSync");
export let mkdtemp = () => _e("mkdtemp");
export let mkdtempSync = () => _e("mkdtempSync");
export let open = () => _e("open");
export let openSync = () => _e("openSync");
export let read = () => _e("read");
export let readSync = () => _e("readSync");
export let promises = new Proxy({}, { get: (name) => _e(`promises/${name}`) });
export let readdir = () => _e("readdir");
export let readdirSync = () => _e("readdirSync");
export let readFile = () => _e("readFile");
export let readFileSync = () => _e("readFileSync");
export let readlink = () => _e("readlink");
export let readlinkSync = () => _e("readlinkSync");
export let realpath = () => _e("realpath");
export let realpathSync = () => _e("realpathSync");
export let rename = () => _e("rename");
export let renameSync = () => _e("renameSync");
export let rmdir = () => _e("rmdir");
export let rmdirSync = () => _e("rmdirSync");
export let rm = () => _e("rm");
export let rmSync = () => _e("rmSync");
export let stat = () => _e("stat");
export let Stats = () => _e("Stats");
export let statSync = () => _e("statSync");
export let symlink = () => _e("symlink");
export let symlinkSync = () => _e("symlinkSync");
export let truncate = () => _e("truncate");
export let truncateSync = () => _e("truncateSync");
export let unlink = () => _e("unlink");
export let unlinkSync = () => _e("unlinkSync");
export let utimes = () => _e("utimes");
export let utimesSync = () => _e("utimesSync");
export let watch = () => _e("watch");
export let watchFile = () => _e("watchFile");
export let write = () => _e("write");
export let writeSync = () => _e("writeSync");
export let writeFile = () => _e("writeFile");
export let writeFileSync = () => _e("writeFileSync");

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
