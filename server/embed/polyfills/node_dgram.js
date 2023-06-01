// https://nodejs.org/api/dgram.html

function panic() {
  throw new Error(
    `[esm.sh] "node:dgram" is not supported in browser environment.`,
  );
}

export class Socket {
  constructor() {
    panic();
  }
}

export function createSocket() {
  panic();
}

export default {
  Socket,
  createSocket,
};
