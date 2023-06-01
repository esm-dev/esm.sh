// https://nodejs.org/api/tls.html

function panic() {
  throw new Error(
    `[esm.sh] "node:tls" is not supported in browser environment.`,
  );
}

export class CryptoStream {
  constructor() {
    panic();
  }
}

export class SecurePair {
  constructor() {
    panic();
  }
}

export class Server {
  constructor() {
    panic();
  }
}

export class TLSSocket {
  constructor() {
    panic();
  }
}

export const rootCertificates = [];
export const DEFAULT_ECDH_CURVE = "auto";
export const DEFAULT_MAX_VERSION = "TLSv1.3";
export const DEFAULT_MIN_VERSION = "TLSv1.2";

export function checkServerIdentity() {
  panic();
}

export function connect() {
  panic();
}

export function createSecureContext() {
  panic();
}

export function createSecurePair() {
  panic();
}

export function createServer() {
  panic();
}

export function getCiphers() {
  panic();
}

export default {
  CryptoStream,
  SecurePair,
  Server,
  TLSSocket,
  rootCertificates,
  DEFAULT_ECDH_CURVE,
  DEFAULT_MAX_VERSION,
  DEFAULT_MIN_VERSION,
  checkServerIdentity,
  connect,
  createSecureContext,
  createSecurePair,
  createServer,
  getCiphers,
};
