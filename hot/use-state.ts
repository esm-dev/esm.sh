import type { Hot } from "../server/embed/types/hot.d.ts";

const doc = document;
const obj = Object;
const symWatch = Symbol();
const symUnWatch = Symbol();
const symAnyKey = Symbol();
const regBlockExpr = /^(!+)?([\w$]+)\s*([\[\^+\-*/%<>|&.!?].+)?$/;

const htmlBuiltinBooleanAttrs = new Set([
  "allowfullscreen",
  "async",
  "autofocus",
  "autoplay",
  "checked",
  "controls",
  "default",
  "defer",
  "disabled",
  "formnovalidate",
  "inert",
  "ismap",
  "itemscope",
  "loop",
  "multiple",
  "muted",
  "nomodule",
  "novalidate",
  "open",
  "playsinline",
  "readonly",
  "required",
  "reversed",
  "selected",
]);

/** create a reactive store. */
function createStore<T extends object>(
  init: T,
  watchInit?: Iterable<[string | symbol, Set<() => void>]>,
): T {
  let filled = false;
  let effectPending = false;
  const effectKeys: Set<string | symbol> = new Set();
  const effect = () => {
    [...effectKeys, symAnyKey].forEach((key) => {
      watchers.get(key)?.forEach((handler) => {
        handler();
      });
    });
    effectKeys.clear();
    effectPending = false;
  };
  const watchers = new Map(watchInit);
  const watch = (key: string | symbol, handler: () => void) => {
    const set = watchers.get(key) ?? (watchers.set(key, new Set()).get(key)!);
    const add = () => set.add(handler);
    add();
    return () => { // dispose
      set.delete(handler);
      return add; // recover
    };
  };
  const unwatch = () => {
    const entries = watchers.entries();
    watchers.clear();
    return entries;
  };
  const store = new Proxy(Array.isArray(init) ? [] : Object.create(null), {
    get: (target, key) => {
      if (key === symWatch) {
        return watch;
      }
      if (key === symUnWatch) {
        return unwatch;
      }
      return get(target, key);
    },
    set: (target, key, value) => {
      const old = get(target, key);
      if (old === value) {
        return true;
      }
      if (isObject(value) && !get(value, symWatch)) {
        let oldWatchers:
          | Iterable<[string | symbol, Set<() => void>]>
          | undefined;
        if (isObject(old)) {
          oldWatchers = get(old, symUnWatch)?.();
        }
        value = createStore(value, oldWatchers);
      }
      const ok = set(target, key, value);
      if (ok && filled) {
        effectKeys.add(key);
        if (!effectPending) {
          effectPending = true;
          // simple scheduler
          queueMicrotask(effect);
        }
      }
      return ok;
    },
  });
  for (const [key, value] of Object.entries(init)) {
    store[key] = value;
  }
  filled = true;
  return store;
}

/** split the given expression by blocks. */
function parseBlocks(expr: string, blockStart = "{", blockEnd = "}") {
  const texts: string[] = [];
  const blocks: string[] = [];
  let i = 0;
  let j = 0;
  while (i < expr.length) {
    j = expr.indexOf(blockStart, i);
    if (j === -1) {
      texts.push(expr.slice(i));
      break;
    }
    texts.push(expr.slice(i, j));
    i = expr.indexOf(blockEnd, j);
    if (i === -1) {
      texts[texts.length - 1] += expr.slice(j);
      break;
    }
    const ident = expr.slice(j + blockStart.length, i).trim();
    if (ident) {
      blocks.push(ident);
    }
    i++;
  }
  return [texts, blocks];
}

/** find the first object that has the given property. */
function findOwn(list: any[], key: PropertyKey) {
  return list.find((o) => Reflect.has(o, key));
}

/** get the given property of the given target. */
function get(target: object, key: PropertyKey) {
  return Reflect.get(target, key);
}

/** set the given property of the given target. */
function set(target: object, key: PropertyKey, value: unknown) {
  return Reflect.set(target, key, value);
}

/** get the attribute value of the given element. */
function attr(el: Element, name: string, value?: string) {
  if (!isNullish(value)) {
    return el.setAttribute(name, value);
  }
  return el.getAttribute(name);
}

/** get all attribute names of the given element. */
function attrs(el: Element) {
  return el.getAttributeNames();
}

/** walk all nodes recursively. */
function walkNodes(
  { childNodes }: Element,
  handler: (node: ChildNode) => void | false,
) {
  childNodes.forEach((node) => {
    if (handler(node) !== false && node.nodeType === 1) {
      walkNodes(node as Element, handler);
    }
  });
}

/** check if the given value is nullish. */
function isNullish(v: unknown): v is null | undefined {
  return v === null || v === undefined;
}

/** check if the given value is an object. */
function isObject(v: unknown): v is object {
  return typeof v === "object" && v !== null;
}

/** check if the given value is a plain object. */
function isPlainObject(v: unknown): v is Record<string, unknown> {
  return isObject(v) && v.constructor === Object;
}

/** check if the given text is a block string. */
function isBlockExpr(text: string, blockStart = "{", blockEnd = "}") {
  const trimed = text.trim();
  return trimed.startsWith(blockStart) && trimed.endsWith(blockEnd);
}

/** convert the given value to string. */
function toString(value: unknown, skipBoolean = false) {
  if (isNullish(value) || (skipBoolean && typeof value === "boolean")) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  return (value as any).toString?.() ?? JSON.stringify(value);
}

/** core logic of the <use-state> tag. */
function core(
  root: Element,
  globalState: Record<string, unknown> | null,
) {
  const inhertScopes = get(root, "$scopes") ??
    (globalState ? [globalState] : []);
  const init = new Function(
    "$scope",
    "return " + (attr(root, "onload") ?? "null"),
  )(
    new Proxy(Object.create(null), {
      get: (_, key) => findOwn(inhertScopes, key)?.[key],
      set: (_, key, value) => {
        const s = findOwn(inhertScopes, key) ?? inhertScopes[0];
        return s ? set(s, key, value) : false;
      },
    }),
  );
  const withProp = attr(root, "with");
  const withScope = withProp
    ? findOwn(inhertScopes, withProp)?.[withProp]
    : null;
  const scopes = [
    ...(isObject(init) ? [createStore(init)] : []),
    ...(isObject(withScope) ? [withScope] : []),
    ...inhertScopes,
  ];
  const interpret = (
    $scopes: Record<string, unknown>[],
    expr: string,
    update: (nextValue: unknown) => void,
    blockStart?: string,
    blockEnd?: string,
  ) => {
    const [texts, blocks] = parseBlocks(expr, blockStart, blockEnd);
    if (blocks.length === 0) {
      return; // no blocks
    }
    const exprCache = new Map<string, string[]>();
    const tokenize = (blockExpr: string) => {
      const expr = exprCache.get(blockExpr);
      if (expr) {
        return expr;
      }
      const m = blockExpr.match(regBlockExpr);
      if (m) {
        exprCache.set(blockExpr, m);
      }
      return m;
    };
    const parse = (expr: string) => {
      const m = tokenize(expr);
      if (m) {
        const [_, op, ident, accesser] = m;
        const scope = findOwn($scopes, ident) ?? $scopes[0];
        return [scope, op, ident, accesser];
      }
      return null;
    };
    const createEffect = (expr: string, callback: (value: unknown) => void) => {
      const ret = parse(expr);
      if (ret) {
        const [scope, op, ident, accesser] = ret;
        let dispose: (() => void) | undefined;
        const invoke = () => {
          dispose?.();
          const call = () => {
            const value = get(scope, ident);
            if (op || accesser) {
              callback(new Function(ident, "return " + expr)(value));
            } else {
              callback(value);
            }
            return value;
          };
          const value = call();
          const shouldWatchCurrentValueToo = accesser &&
            (accesser.startsWith(".") || accesser.startsWith("[")) &&
            isObject(value);
          if (shouldWatchCurrentValueToo) {
            dispose = get(value, symWatch)(symAnyKey, call);
          }
        };
        invoke();
        scope[symWatch]?.(ident, invoke);
      }
    };
    // singleton block
    if (blocks.length === 1 && isBlockExpr(expr)) {
      return createEffect(blocks[0], update);
    }
    const invokedBlocks = new Array(blocks.length);
    const merge = () => {
      const mergedValue = texts.map((text, i) => {
        if (blocks[i]) {
          return text + toString(invokedBlocks[i], true);
        }
        return text;
      }).join("");
      update(mergedValue);
    };
    let firstMerge = false;
    blocks.map((blockExpr, i) => {
      createEffect(blockExpr, (value) => {
        invokedBlocks[i] = value;
        if (firstMerge) {
          merge();
        }
      });
    });
    merge();
    firstMerge = true;
  };
  const reactive = (el: Element, currentScope?: unknown) => {
    let $scopes = scopes;
    if (currentScope) {
      $scopes = [currentScope, ...scopes];
    }
    const handler = (node: ChildNode) => {
      if (node.nodeType === 1 /* element node */) {
        const el = node as Element;
        const tagName = el.tagName.toLowerCase();
        const props = attrs(el);

        // nested <use-state> tag
        if (tagName === "use-state") {
          Object.assign(el, { $scopes });
          return false;
        }

        const boolAttrs = new Set<string>();
        const commonAttrs = new Set<string>();
        const eventAttrs = new Set<string>();

        for (const prop of props) {
          if (attr(el, prop) === "" && !htmlBuiltinBooleanAttrs.has(prop)) {
            boolAttrs.add(prop);
          } else if (prop.startsWith("on")) {
            eventAttrs.add(prop);
          } else {
            commonAttrs.add(prop);
          }
        }

        // list rendering
        if (commonAttrs.has("for")) {
          const [iter, key] = attr(el, "for")!.split(" of ").map((s) =>
            s.trim()
          );
          if (iter && key) {
            const templateEl = el;
            const keyProp = attr(templateEl, "key");
            const placeholder = doc!.createComment("&")!;
            const scope = findOwn($scopes, key) ?? $scopes[0];
            let marker: Element[] = [];
            let unwatch: (() => void) | undefined;
            const renderList = () => {
              unwatch?.(); // dispose the previous array watcher if exists
              const arr = get(scope, key);
              if (Array.isArray(arr)) {
                let iterKey = iter;
                let iterIndex = "";
                if (isBlockExpr(iterKey, "(", ")")) {
                  [iterKey, iterIndex] = iterKey.slice(1, -1)
                    .split(",", 2)
                    .map((s) => s.trim());
                }
                const render = () => {
                  const map = new Map<string, Element>();
                  for (const el of marker) {
                    const key = attr(el, "key");
                    key && map.set(key, el);
                  }
                  const listEls = arr.map((item, index) => {
                    const iterScope: Record<string, unknown> = {};
                    if (iterKey) {
                      iterScope[iterKey] = item;
                    }
                    if (iterIndex) {
                      iterScope[iterIndex] = index;
                    }
                    if (keyProp && map.size > 0) {
                      let key = "";
                      interpret(
                        [iterScope, ...$scopes],
                        keyProp,
                        (ret) => {
                          key = toString(ret);
                        },
                      );
                      const sameKeyEl = map.get(key);
                      if (sameKeyEl) {
                        if (iterIndex) {
                          get(sameKeyEl, "$scope")[iterIndex] = index;
                        }
                        return sameKeyEl;
                      }
                    }
                    const listEl = templateEl.cloneNode(true) as Element;
                    const lterScopeStore = createStore(iterScope);
                    set(listEl, "$scope", lterScopeStore);
                    reactive(listEl, lterScopeStore);
                    return listEl;
                  });
                  marker.forEach((el) => el.remove());
                  listEls.forEach((el) => placeholder.before(el));
                  marker = listEls;
                };
                render();
                // watch the array changes and re-render
                unwatch = get(arr, symWatch)(symAnyKey, render);
              } else if (marker.length > 0) {
                marker.forEach((el) => el.remove());
                marker.length = 0;
              }
            };
            el.replaceWith(placeholder);
            templateEl.removeAttribute("for");
            renderList();
            scope[symWatch](key, renderList);
          }
          return false;
        }

        // render properties with state
        const style = [attr(el, "style"), null];
        for (const prop of commonAttrs) {
          const isStyle = prop === "style";
          interpret(
            $scopes,
            attr(el, prop) ?? "",
            (v) => {
              if (htmlBuiltinBooleanAttrs.has(prop)) {
                if (v) {
                  attr(el, prop, "");
                } else {
                  el.removeAttribute(prop);
                }
              } else {
                let propName = prop;
                let propValue = toString(v, true);
                if (isStyle) {
                  style[0] = propValue;
                } else if (propName === "+style") {
                  style[1] = propValue;
                  propName = "style";
                }
                if (propName === "style") {
                  propValue = style.filter(Boolean).join(";");
                }
                attr(el, propName, propValue);
              }
            },
            isStyle ? "state(" : undefined,
            isStyle ? ")" : undefined,
          );
        }

        // conditional rendering
        let cProp = "";
        let notOp = false;
        for (let prop of boolAttrs) {
          notOp = prop.startsWith("!");
          if (notOp) {
            prop = prop.slice(1);
          }
          if (findOwn($scopes, prop)) {
            cProp = prop;
            break;
          }
        }
        if (cProp) {
          const scope = findOwn($scopes, cProp);
          if (scope) {
            const cEl = el;
            const placeholder = doc!.createComment("&")!;
            let anchor: ChildNode = el;
            const switchEl = (nextEl: ChildNode) => {
              if (nextEl !== anchor) {
                anchor.replaceWith(nextEl);
                anchor = nextEl;
              }
            };
            const toggle = () => {
              let ok = get(scope!, cProp!);
              if (notOp) {
                ok = !ok;
              }
              if (ok) {
                switchEl(cEl);
              } else {
                switchEl(placeholder);
              }
            };
            toggle();
            scope[symWatch](cProp, toggle);
          }
        }

        // bind scopes for event handlers
        if (eventAttrs.size > 0) {
          const marker = new Set<string>();
          for (const scope of $scopes) {
            const keys = obj.keys(scope);
            for (const key of keys) {
              if (!marker.has(key)) {
                marker.add(key);
                if (!Object.hasOwn(el, key)) {
                  Object.defineProperty(el, key, {
                    get: () => get(scope, key),
                    set: (value) => set(scope, key, value),
                  });
                }
              }
            }
          }
          // apply event modifiers if exists
          for (const a of eventAttrs) {
            let handler = attr(el, a);
            if (handler) {
              const [event, ...rest] = a.toLowerCase().split(".");
              const modifiers = new Set(rest);
              const addCode = (code: string) => {
                handler = code + handler;
              };
              if (modifiers.size) {
                if (modifiers.delete("once")) {
                  addCode(`this.removeAttribute('${event}');`);
                }
                if (modifiers.delete("prevent")) {
                  addCode("event.preventDefault();");
                }
                if (modifiers.delete("stop")) {
                  addCode("event.stopPropagation();");
                }
                if (event.startsWith("onkey") && modifiers.size) {
                  if (modifiers.delete("space")) {
                    modifiers.add(" ");
                  }
                  addCode(
                    "if(![" +
                      ([...modifiers].map((k) =>
                        "'" + (k.length > 1
                          ? k.charAt(0).toUpperCase() + k.slice(1)
                          : k) +
                        "'"
                      ).join(",")) +
                      "].includes(event.key))return;",
                  );
                }
                el.removeAttribute(a);
                attr(el, event, handler);
              }
            }
          }
        }

        // bind state for input, select and textarea elements
        if (
          tagName === "input" ||
          tagName === "select" ||
          tagName === "textarea"
        ) {
          const inputEl = el as
            | HTMLInputElement
            | HTMLSelectElement
            | HTMLTextAreaElement;
          const name = attr(inputEl, "name");
          if (name) {
            const scope = findOwn($scopes, name);
            if (scope) {
              const type = attr(inputEl, "type");
              const isCheckBox = type === "checkbox";
              const update = () => {
                const value = get(scope, name);
                if (isCheckBox) {
                  (inputEl as HTMLInputElement).checked = !!value;
                } else {
                  inputEl.value = toString(value);
                }
              };
              let convert: (input: string) => unknown = String;
              if (type === "number") {
                convert = Number;
              } else if (isCheckBox) {
                convert = (_input) => (inputEl as HTMLInputElement).checked;
              }
              update();
              const dispose = scope[symWatch](name, update);
              inputEl.addEventListener("input", () => {
                const recover = dispose();
                set(scope, name, convert(inputEl.value));
                recover();
              });
            }
          }
        }
      } else if (node.nodeType === 3 /* text node */) {
        interpret(
          $scopes,
          node.textContent ?? "",
          (v) => node.textContent = toString(v, true),
        );
      }
    };
    if (el !== root) {
      handler(el);
    }
    walkNodes(el, handler);
  };
  reactive(root);
}

const plugin = {
  name: "use-state",
  setup(hot: Hot) {
    let globalState: Record<string, unknown> | null = null;

    hot.state = (
      init: Record<string, unknown> | Promise<Record<string, unknown>>,
    ): void => {
      if (init instanceof Promise) {
        hot.waitUntil(init.then((state) => hot.state(state)));
      } else if (isPlainObject(init)) {
        globalState = createStore(init);
      }
    };

    hot.onFire(() => {
      customElements.define(
        "use-state",
        class extends HTMLElement {
          connectedCallback() {
            core(this, globalState);
          }
        },
      );
      doc!.head.appendChild(doc!.createElement("style")).append(
        "use-state{visibility: visible;}",
      );
    });
  },
};

// use the plugin as a standalone module:
// https://esm.sh/hot/use-state?standalone
export function standalone() {
  const hot = {
    _promises: [] as Promise<void>[],
    _fire: () => {},
    waitUntil(promise: Promise<void>) {
      this._promises.push(promise);
    },
    onFire(fn: () => void) {
      this._fire = fn;
    },
  };
  plugin.setup(hot as unknown as Hot);
  return (
    init?: Record<string, unknown> | Promise<Record<string, unknown>>,
  ) => {
    init && (hot as unknown as Hot).state(init);
    if (hot._promises.length > 0) {
      Promise.all(hot._promises).then(() => hot._fire());
    } else {
      hot._fire();
    }
  };
}

export default plugin;
