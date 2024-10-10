// https://nodejs.org/api/perf_hooks.html

function panic() {
  throw new Error(
    `[esm.sh] "node:perf_hooks" is not supported in browser environment.`,
  );
}

export const performance = globalThis.performance;
export const PerformanceObserver = globalThis.PerformanceObserver;
export const PerformanceEntry = globalThis.PerformanceEntry;
export const PerformanceObserverEntryList = globalThis.PerformanceObserverEntryList;

export class PerformanceNodeTiming extends PerformanceEntry {
  constructor() {
    panic();
  }
}

export class Histogram {
  constructor() {
    panic();
  }
}

export class IntervalHistogram extends Histogram {
  constructor() {
    panic();
  }
}

export class RecordableHistogram extends Histogram {
  constructor() {
    panic();
  }
}

export function createHistogram() {
  panic();
}

export function monitorEventLoopDelay() {
  panic();
}

export default {
  performance,
  PerformanceEntry,
  PerformanceNodeTiming,
  PerformanceObserver,
  PerformanceObserverEntryList,
  Histogram,
  IntervalHistogram,
  RecordableHistogram,
  createHistogram,
  monitorEventLoopDelay,
};
