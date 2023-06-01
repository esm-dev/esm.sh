// https://nodejs.org/api/worker_threads.html

function panic() {
  throw new Error(
    `[esm.sh] "node:worker_threads" is not supported in browser environment.`,
  );
}

export const isMainThread = true;
export const parentPort = null;
export const resourceLimits = {
  maxYoungGenerationSizeMb: 0,
  maxOldGenerationSizeMb: 0,
  codeRangeSizeMb: 0,
  stackSizeMb: 0,
};
export const SHARE_ENV = Symbol("worker_threads.SHARE_ENV");
export const threadId = 0;
export const workerData = undefined;

export function getEnvironmentData() {
  panic();
}

export function markAsUntransferable() {
  panic();
}

export function moveMessagePortToContext() {
  panic();
}

export function receiveMessageOnPort() {
  panic();
}

export function setEnvironmentData() {
  panic();
}

export class BroadcastChannel {
  constructor() {
    panic();
  }
}

export class MessageChannel {
  constructor() {
    panic();
  }
}

export class MessagePort {
  constructor() {
    panic();
  }
}

export class Worker {
  constructor() {
    panic();
  }
}

export default {
  isMainThread,
  parentPort,
  resourceLimits,
  SHARE_ENV,
  threadId,
  workerData,
  getEnvironmentData,
  markAsUntransferable,
  moveMessagePortToContext,
  receiveMessageOnPort,
  setEnvironmentData,
  BroadcastChannel,
  MessageChannel,
  MessagePort,
  Worker,
};
