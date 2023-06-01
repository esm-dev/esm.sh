// https://nodejs.org/api/net.html

function panic() {
  throw new Error(
    `[esm.sh] "node:net" is not supported in browser environment.`,
  );
}

export class BlockList {
  constructor() {
    panic();
  }
}

export class SocketAddress {
  constructor() {
    panic();
  }
}

export class Server {
  constructor() {
    panic();
  }
}

export class Socket {
  constructor() {
    panic();
  }
}

export function connect() {
  panic();
}

export function createConnection() {
  panic();
}

export function createServer() {
  panic();
}

export function getDefaultAutoSelectFamily() {
  panic();
}

export function setDefaultAutoSelectFamily() {
  panic();
}

export function getDefaultAutoSelectFamilyAttemptTimeout() {
  panic();
}

export function setDefaultAutoSelectFamilyAttemptTimeout() {
  panic();
}

export function isIP(addr) {
  if (isIPv4(addr)) return 4;
  if (isIPv6(addr)) return 6;
  return 0;
}

export function isIPv4(addr) {
  if (typeof addr !== "string") return false;
  const parts = addr.split(".");
  if (parts.length !== 4) return false;
  for (const part of parts) {
    const n = Number(part);
    if (Number.isNaN(n) || n < 0 || n > 255) return false;
  }
  return true;
}

export function isIPv6() {
  if (typeof addr !== "string") return false;
  const parts = addr.split(":");
  if (parts.length < 3 || parts.length > 8) return false;
  for (const part of parts) {
    if (part.length === 0) return false;
    if (part.length > 4) return false;
    if (!/^[0-9a-fA-F]+$/.test(part)) return false;
  }
  return true;
}

export default {
  BlockList,
  SocketAddress,
  Server,
  Socket,
  connect,
  createConnection,
  createServer,
  getDefaultAutoSelectFamily,
  setDefaultAutoSelectFamily,
  getDefaultAutoSelectFamilyAttemptTimeout,
  setDefaultAutoSelectFamilyAttemptTimeout,
  isIP,
  isIPv4,
  isIPv6,
};
