import { promises } from './node_fs.js';

export const {
  access,
  copyFile,
  cp,
  open,
  opendir,
  rename,
  truncate,
  rm,
  rmdir,
  mkdir,
  readdir,
  readlink,
  symlink,
  lstat,
  stat,
  link,
  unlink,
  chmod,
  lchmod,
  lchown,
  chown,
  utimes,
  realpath,
  mkdtemp,
  writeFile,
  appendFile,
  readFile,
  watch,
} = promises;

export default promises;
