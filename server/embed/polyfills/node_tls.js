// https://nodejs.org/api/tls.html

function notImplemented(name) {
  throw new Error(`[esm.sh] tls: '${name}' is not implemented`)
}

export class CryptoStream {
  constructor() {
    notImplemented('CryptoStream')
  }
}

export class SecurePair {
  constructor() {
    notImplemented('SecurePair')
  }
}

export class Server {
  constructor() {
    notImplemented('Server')
  }
}

export class TLSSocket {
  constructor() {
    notImplemented('TLSSocket')
  }
}

export const rootCertificates = []
export const DEFAULT_ECDH_CURVE = 'auto'
export const DEFAULT_MAX_VERSION = 'TLSv1.3'
export const DEFAULT_MIN_VERSION = 'TLSv1.2'

export function checkServerIdentity() {
  notImplemented('checkServerIdentity')
}

export function connect() {
  notImplemented('connect')
}

export function createSecureContext() {
  notImplemented('createSecureContext')
}

export function createSecurePair() {
  notImplemented('createSecurePair')
}

export function createServer() {
  notImplemented('createServer')
}


export function getCiphers() {
  notImplemented('getCiphers')
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
}
