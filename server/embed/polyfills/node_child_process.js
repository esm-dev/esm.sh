// https://nodejs.org/api/child_process.html

function panic() {
  throw new Error(
    `[esm.sh] "node:child_process" is not supported in browser environment.`,
  );
}

export const _forkChild = panic;
export const ChildProcess = panic;
export const exec = panic;
export const execFile = panic;
export const execFileSync = panic;
export const execSync = panic;
export const fork = panic;
export const spawn = panic;
export const spawnSync = panic;

export default {
  _forkChild,
  ChildProcess,
  exec,
  execFile,
  execFileSync,
  execSync,
  fork,
  spawn,
  spawnSync,
};
