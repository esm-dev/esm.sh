import uaParser from "ua-parser-js";

type Version = [number, number, number];

/** possible targets */
export const targets = new Set([
  "es2015",
  "es2016",
  "es2017",
  "es2018",
  "es2019",
  "es2020",
  "es2021",
  "es2022",
  "esnext",
  "deno",
  "denonext",
  "node",
]);

/** the js table transpiled from https://github.com/evanw/esbuild/blob/main/internal/compat/js_table.go */
const jsTable: Record<string, Record<string, Version>> = {
  ArbitraryModuleNamespaceNames: {
    Chrome: [90, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [87, 0, 0],
    IOS: [14, 5, 0],
    Node: [16, 0, 0],
    Safari: [14, 1, 0],
  },
  ArraySpread: {
    // Note: The latest version of "IE" failed 15 tests including: spread syntax for iterable objects: spreading non-iterables is a runtime error
    // Note: The latest version of "Rhino" failed 15 tests including: spread syntax for iterable objects: spreading non-iterables is a runtime error
    Chrome: [46, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [36, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 0, 0],
    Node: [5, 0, 0],
    Opera: [33, 0, 0],
    Safari: [10, 0, 0],
  },
  Arrow: {
    // Note: The latest version of "Hermes" failed 3 tests including: arrow functions: lexical "super" binding in constructors
    // Note: The latest version of "IE" failed 13 tests including: arrow functions: "this" unchanged by call or apply
    // Note: The latest version of "Rhino" failed 3 tests including: arrow functions: lexical "new.target" binding
    Chrome: [49, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [45, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 0, 0],
    Opera: [36, 0, 0],
    Safari: [10, 0, 0],
  },
  AsyncAwait: {
    // Note: The latest version of "Hermes" failed 4 tests including: async functions: async arrow functions
    // Note: The latest version of "IE" failed 16 tests including: async functions: async arrow functions
    // Note: The latest version of "Rhino" failed 16 tests including: async functions: async arrow functions
    Chrome: [55, 0, 0],
    Deno: [1, 0, 0],
    Edge: [15, 0, 0],
    ES: [2017, 0, 0],
    Firefox: [52, 0, 0],
    IOS: [11, 0, 0],
    Node: [7, 6, 0],
    Opera: [42, 0, 0],
    Safari: [11, 0, 0],
  },
  AsyncGenerator: {
    // Note: The latest version of "Hermes" failed this test: Asynchronous Iterators: async generators
    // Note: The latest version of "IE" failed this test: Asynchronous Iterators: async generators
    // Note: The latest version of "Rhino" failed this test: Asynchronous Iterators: async generators
    Chrome: [63, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [57, 0, 0],
    IOS: [12, 0, 0],
    Node: [10, 0, 0],
    Opera: [50, 0, 0],
    Safari: [12, 0, 0],
  },
  Bigint: {
    // Note: The latest version of "IE" failed this test: BigInt: basic functionality
    Chrome: [67, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [68, 0, 0],
    Hermes: [0, 12, 0],
    IOS: [14, 0, 0],
    Node: [10, 4, 0],
    Opera: [54, 0, 0],
    Rhino: [1, 7, 14],
    Safari: [14, 0, 0],
  },
  Class: {
    // Note: The latest version of "Hermes" failed 24 tests including: class: accessor properties
    // Note: The latest version of "IE" failed 24 tests including: class: accessor properties
    // Note: The latest version of "Rhino" failed 24 tests including: class: accessor properties
    Chrome: [49, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [45, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 0, 0],
    Opera: [36, 0, 0],
    Safari: [10, 0, 0],
  },
  ClassField: {
    // Note: The latest version of "Hermes" failed 2 tests including: instance class fields: computed instance class fields
    // Note: The latest version of "IE" failed 2 tests including: instance class fields: computed instance class fields
    // Note: The latest version of "Rhino" failed 2 tests including: instance class fields: computed instance class fields
    Chrome: [73, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [69, 0, 0],
    IOS: [14, 0, 0],
    Node: [12, 0, 0],
    Opera: [60, 0, 0],
    Safari: [14, 0, 0],
  },
  ClassPrivateAccessor: {
    // Note: The latest version of "Hermes" failed this test: private class methods: private accessor properties
    // Note: The latest version of "IE" failed this test: private class methods: private accessor properties
    // Note: The latest version of "Rhino" failed this test: private class methods: private accessor properties
    Chrome: [84, 0, 0],
    Deno: [1, 0, 0],
    Edge: [84, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [15, 0, 0],
    Node: [14, 6, 0],
    Opera: [70, 0, 0],
    Safari: [15, 0, 0],
  },
  ClassPrivateBrandCheck: {
    // Note: The latest version of "Hermes" failed this test: Ergonomic brand checks for private fields
    // Note: The latest version of "IE" failed this test: Ergonomic brand checks for private fields
    // Note: The latest version of "Rhino" failed this test: Ergonomic brand checks for private fields
    Chrome: [91, 0, 0],
    Deno: [1, 9, 0],
    Edge: [91, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [15, 0, 0],
    Node: [16, 4, 0],
    Opera: [77, 0, 0],
    Safari: [15, 0, 0],
  },
  ClassPrivateField: {
    // Note: The latest version of "Hermes" failed 4 tests including: instance class fields: optional deep private instance class fields access
    // Note: The latest version of "IE" failed 4 tests including: instance class fields: optional deep private instance class fields access
    // Note: The latest version of "Rhino" failed 4 tests including: instance class fields: optional deep private instance class fields access
    Chrome: [84, 0, 0],
    Deno: [1, 0, 0],
    Edge: [84, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [14, 5, 0],
    Node: [14, 6, 0],
    Opera: [70, 0, 0],
    Safari: [14, 1, 0],
  },
  ClassPrivateMethod: {
    // Note: The latest version of "Hermes" failed this test: private class methods: private instance methods
    // Note: The latest version of "IE" failed this test: private class methods: private instance methods
    // Note: The latest version of "Rhino" failed this test: private class methods: private instance methods
    Chrome: [84, 0, 0],
    Deno: [1, 0, 0],
    Edge: [84, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [15, 0, 0],
    Node: [14, 6, 0],
    Opera: [70, 0, 0],
    Safari: [15, 0, 0],
  },
  ClassPrivateStaticAccessor: {
    // Note: The latest version of "Hermes" failed this test: private class methods: private static accessor properties
    // Note: The latest version of "IE" failed this test: private class methods: private static accessor properties
    // Note: The latest version of "Rhino" failed this test: private class methods: private static accessor properties
    Chrome: [84, 0, 0],
    Deno: [1, 0, 0],
    Edge: [84, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [15, 0, 0],
    Node: [14, 6, 0],
    Opera: [70, 0, 0],
    Safari: [15, 0, 0],
  },
  ClassPrivateStaticField: {
    // Note: The latest version of "Hermes" failed this test: static class fields: private static class fields
    // Note: The latest version of "IE" failed this test: static class fields: private static class fields
    // Note: The latest version of "Rhino" failed this test: static class fields: private static class fields
    Chrome: [74, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [14, 5, 0],
    Node: [12, 0, 0],
    Opera: [62, 0, 0],
    Safari: [14, 1, 0],
  },
  ClassPrivateStaticMethod: {
    // Note: The latest version of "Hermes" failed this test: private class methods: private static methods
    // Note: The latest version of "IE" failed this test: private class methods: private static methods
    // Note: The latest version of "Rhino" failed this test: private class methods: private static methods
    Chrome: [84, 0, 0],
    Deno: [1, 0, 0],
    Edge: [84, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [90, 0, 0],
    IOS: [15, 0, 0],
    Node: [14, 6, 0],
    Opera: [70, 0, 0],
    Safari: [15, 0, 0],
  },
  ClassStaticBlocks: {
    Chrome: [91, 0, 0],
    Deno: [1, 14, 0],
    Edge: [94, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [93, 0, 0],
    IOS: [16, 4, 0],
    Node: [16, 11, 0],
    Opera: [80, 0, 0],
    Safari: [16, 4, 0],
  },
  ClassStaticField: {
    // Note: The latest version of "Hermes" failed 2 tests including: static class fields: computed static class fields
    // Note: The latest version of "IE" failed 2 tests including: static class fields: computed static class fields
    // Note: The latest version of "Rhino" failed 2 tests including: static class fields: computed static class fields
    Chrome: [73, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [75, 0, 0],
    IOS: [14, 5, 0],
    Node: [12, 0, 0],
    Opera: [60, 0, 0],
    Safari: [14, 1, 0],
  },
  ConstAndLet: {
    // Note: The latest version of "Hermes" failed 20 tests including: const: for loop statement scope
    // Note: The latest version of "IE" failed 6 tests including: const: for-in loop iteration scope
    // Note: The latest version of "Rhino" failed 22 tests including: const: cannot be in statements
    Chrome: [49, 0, 0],
    Deno: [1, 0, 0],
    Edge: [14, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [51, 0, 0],
    IOS: [11, 0, 0],
    Node: [6, 0, 0],
    Opera: [36, 0, 0],
    Safari: [11, 0, 0],
  },
  Decorators: {},
  DefaultArgument: {
    // Note: The latest version of "Hermes" failed 2 tests including: default function parameters: separate scope
    // Note: The latest version of "IE" failed 7 tests including: default function parameters: arguments object interaction
    // Note: The latest version of "Rhino" failed 7 tests including: default function parameters: arguments object interaction
    Chrome: [49, 0, 0],
    Deno: [1, 0, 0],
    Edge: [14, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [53, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 0, 0],
    Opera: [36, 0, 0],
    Safari: [10, 0, 0],
  },
  Destructuring: {
    // Note: The latest version of "Hermes" failed 3 tests including: destructuring, declarations: defaults, let temporal dead zone
    // Note: The latest version of "IE" failed 71 tests including: destructuring, assignment: chained iterable destructuring
    // Note: The latest version of "Rhino" failed 33 tests including: destructuring, assignment: computed properties
    Chrome: [51, 0, 0],
    Deno: [1, 0, 0],
    Edge: [18, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [53, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 5, 0],
    Opera: [38, 0, 0],
    Safari: [10, 0, 0],
  },
  DynamicImport: {
    Chrome: [63, 0, 0],
    Edge: [79, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [67, 0, 0],
    IOS: [11, 0, 0],
    Node: [13, 2, 0],
    Opera: [50, 0, 0],
    Safari: [11, 1, 0],
  },
  ExponentOperator: {
    // Note: The latest version of "IE" failed 3 tests including: exponentiation (**) operator: assignment
    Chrome: [52, 0, 0],
    Deno: [1, 0, 0],
    Edge: [14, 0, 0],
    ES: [2016, 0, 0],
    Firefox: [52, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 3, 0],
    Node: [7, 0, 0],
    Opera: [39, 0, 0],
    Rhino: [1, 7, 14],
    Safari: [10, 1, 0],
  },
  ExportStarAs: {
    Chrome: [72, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [80, 0, 0],
    IOS: [14, 5, 0],
    Node: [13, 2, 0],
    Opera: [60, 0, 0],
    Safari: [14, 1, 0],
  },
  ForAwait: {
    // Note: The latest version of "Hermes" failed this test: Asynchronous Iterators: for-await-of loops
    // Note: The latest version of "IE" failed this test: Asynchronous Iterators: for-await-of loops
    // Note: The latest version of "Rhino" failed this test: Asynchronous Iterators: for-await-of loops
    Chrome: [63, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [57, 0, 0],
    IOS: [12, 0, 0],
    Node: [10, 0, 0],
    Opera: [50, 0, 0],
    Safari: [12, 0, 0],
  },
  ForOf: {
    // Note: The latest version of "IE" failed 9 tests including: for..of loops: iterator closing, break
    // Note: The latest version of "Rhino" failed 4 tests including: for..of loops: iterator closing, break
    Chrome: [51, 0, 0],
    Deno: [1, 0, 0],
    Edge: [15, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [53, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 0, 0],
    Node: [6, 5, 0],
    Opera: [38, 0, 0],
    Safari: [10, 0, 0],
  },
  FunctionNameConfigurable: {
    // Note: The latest version of "IE" failed this test: function "name" property: isn't writable, is configurable
    // Note: The latest version of "Rhino" failed this test: function "name" property: isn't writable, is configurable
    Chrome: [43, 0, 0],
    Deno: [1, 0, 0],
    Edge: [12, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [38, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 0, 0],
    Node: [4, 0, 0],
    Opera: [30, 0, 0],
    Safari: [10, 0, 0],
  },
  FunctionOrClassPropertyAccess: {
    Chrome: [0, 0, 0],
    Deno: [0, 0, 0],
    Edge: [0, 0, 0],
    ES: [0, 0, 0],
    Firefox: [0, 0, 0],
    Hermes: [0, 0, 0],
    IE: [0, 0, 0],
    IOS: [0, 0, 0],
    Node: [0, 0, 0],
    Opera: [0, 0, 0],
    Rhino: [0, 0, 0],
    Safari: [16, 3, 0],
  },
  Generator: {
    // Note: The latest version of "Hermes" failed 3 tests including: generators: computed shorthand generators, classes
    // Note: The latest version of "IE" failed 27 tests including: generators: %GeneratorPrototype%
    // Note: The latest version of "Rhino" failed 15 tests including: generators: %GeneratorPrototype%
    Chrome: [50, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [53, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 0, 0],
    Opera: [37, 0, 0],
    Safari: [10, 0, 0],
  },
  Hashbang: {
    // Note: The latest version of "IE" failed this test: Hashbang Grammar
    // Note: The latest version of "Rhino" failed this test: Hashbang Grammar
    Chrome: [74, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    Firefox: [67, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [13, 4, 0],
    Node: [12, 5, 0],
    Opera: [62, 0, 0],
    Safari: [13, 1, 0],
  },
  ImportAssertions: {
    Chrome: [91, 0, 0],
    Deno: [1, 17, 0],
    Edge: [91, 0, 0],
    Node: [16, 14, 0],
  },
  ImportAttributes: {
    Deno: [1, 37, 0],
    Node: [20, 10, 0],
  },
  ImportMeta: {
    Chrome: [64, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [62, 0, 0],
    IOS: [12, 0, 0],
    Node: [10, 4, 0],
    Opera: [51, 0, 0],
    Safari: [11, 1, 0],
  },
  InlineScript: {},
  LogicalAssignment: {
    // Note: The latest version of "IE" failed 9 tests including: Logical Assignment: &&= basic support
    // Note: The latest version of "Rhino" failed 9 tests including: Logical Assignment: &&= basic support
    Chrome: [85, 0, 0],
    Deno: [1, 2, 0],
    Edge: [85, 0, 0],
    ES: [2021, 0, 0],
    Firefox: [79, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [14, 0, 0],
    Node: [15, 0, 0],
    Opera: [71, 0, 0],
    Safari: [14, 0, 0],
  },
  NestedRestBinding: {
    // Note: The latest version of "IE" failed 2 tests including: nested rest destructuring, declarations
    // Note: The latest version of "Rhino" failed 2 tests including: nested rest destructuring, declarations
    Chrome: [49, 0, 0],
    Deno: [1, 0, 0],
    Edge: [14, 0, 0],
    ES: [2016, 0, 0],
    Firefox: [47, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 3, 0],
    Node: [6, 0, 0],
    Opera: [36, 0, 0],
    Safari: [10, 1, 0],
  },
  NewTarget: {
    // Note: The latest version of "IE" failed 2 tests including: new.target: assignment is an early error
    // Note: The latest version of "Rhino" failed 2 tests including: new.target: assignment is an early error
    Chrome: [46, 0, 0],
    Deno: [1, 0, 0],
    Edge: [14, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [41, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 0, 0],
    Node: [5, 0, 0],
    Opera: [33, 0, 0],
    Safari: [10, 0, 0],
  },
  NodeColonPrefixImport: {
    Node: [14, 13, 1],
  },
  NodeColonPrefixRequire: {
    Node: [16, 0, 0],
  },
  NullishCoalescing: {
    // Note: The latest version of "IE" failed this test: nullish coalescing operator (??)
    // Note: The latest version of "Rhino" failed this test: nullish coalescing operator (??)
    Chrome: [80, 0, 0],
    Deno: [1, 0, 0],
    Edge: [80, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [72, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [13, 4, 0],
    Node: [14, 0, 0],
    Opera: [67, 0, 0],
    Safari: [13, 1, 0],
  },
  ObjectAccessors: {
    Chrome: [5, 0, 0],
    Deno: [1, 0, 0],
    Edge: [12, 0, 0],
    ES: [5, 0, 0],
    Firefox: [2, 0, 0],
    Hermes: [0, 7, 0],
    IE: [9, 0, 0],
    IOS: [6, 0, 0],
    Node: [0, 4, 0],
    Opera: [10, 10, 0],
    Rhino: [1, 7, 13],
    Safari: [3, 1, 0],
  },
  ObjectExtensions: {
    // Note: The latest version of "IE" failed 6 tests including: object literal extensions: computed accessors
    // Note: The latest version of "Rhino" failed 3 tests including: object literal extensions: computed accessors
    Chrome: [44, 0, 0],
    Deno: [1, 0, 0],
    Edge: [12, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [34, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [10, 0, 0],
    Node: [4, 0, 0],
    Opera: [31, 0, 0],
    Safari: [10, 0, 0],
  },
  ObjectRestSpread: {
    // Note: The latest version of "IE" failed 2 tests including: object rest/spread properties: object rest properties
    // Note: The latest version of "Rhino" failed 2 tests including: object rest/spread properties: object rest properties
    Chrome: [60, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [55, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [11, 3, 0],
    Node: [8, 3, 0],
    Opera: [47, 0, 0],
    Safari: [11, 1, 0],
  },
  OptionalCatchBinding: {
    // Note: The latest version of "IE" failed 3 tests including: optional catch binding: await
    // Note: The latest version of "Rhino" failed 3 tests including: optional catch binding: await
    Chrome: [66, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2019, 0, 0],
    Firefox: [58, 0, 0],
    Hermes: [0, 12, 0],
    IOS: [11, 3, 0],
    Node: [10, 0, 0],
    Opera: [53, 0, 0],
    Safari: [11, 1, 0],
  },
  OptionalChain: {
    // Note: The latest version of "IE" failed 5 tests including: optional chaining operator (?.): optional bracket access
    // Note: The latest version of "Rhino" failed 5 tests including: optional chaining operator (?.): optional bracket access
    Chrome: [91, 0, 0],
    Deno: [1, 9, 0],
    Edge: [91, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [74, 0, 0],
    Hermes: [0, 12, 0],
    IOS: [13, 4, 0],
    Node: [16, 1, 0],
    Opera: [77, 0, 0],
    Safari: [13, 1, 0],
  },
  RegexpDotAllFlag: {
    // Note: The latest version of "IE" failed this test: s (dotAll) flag for regular expressions
    // Note: The latest version of "Rhino" failed this test: s (dotAll) flag for regular expressions
    Chrome: [62, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [78, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [11, 3, 0],
    Node: [8, 10, 0],
    Opera: [49, 0, 0],
    Safari: [11, 1, 0],
  },
  RegexpLookbehindAssertions: {
    // Note: The latest version of "IE" failed this test: RegExp Lookbehind Assertions
    // Note: The latest version of "Rhino" failed this test: RegExp Lookbehind Assertions
    Chrome: [62, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [78, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [16, 4, 0],
    Node: [8, 10, 0],
    Opera: [49, 0, 0],
    Safari: [16, 4, 0],
  },
  RegexpMatchIndices: {
    Chrome: [90, 0, 0],
    Deno: [1, 8, 0],
    Edge: [90, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [88, 0, 0],
    IOS: [15, 0, 0],
    Node: [16, 0, 0],
    Opera: [76, 0, 0],
    Safari: [15, 0, 0],
  },
  RegexpNamedCaptureGroups: {
    // Note: The latest version of "Hermes" failed this test: RegExp named capture groups
    // Note: The latest version of "IE" failed this test: RegExp named capture groups
    // Note: The latest version of "Rhino" failed this test: RegExp named capture groups
    Chrome: [64, 0, 0],
    Deno: [1, 0, 0],
    Edge: [79, 0, 0],
    ES: [2018, 0, 0],
    Firefox: [78, 0, 0],
    IOS: [11, 3, 0],
    Node: [10, 0, 0],
    Opera: [51, 0, 0],
    Safari: [11, 1, 0],
  },
  RegexpSetNotation: {},
  RegexpStickyAndUnicodeFlags: {
    // Note: The latest version of "IE" failed 6 tests including: RegExp "y" and "u" flags: "u" flag
    // Note: The latest version of "Rhino" failed 6 tests including: RegExp "y" and "u" flags: "u" flag
    Chrome: [50, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [46, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [12, 0, 0],
    Node: [6, 0, 0],
    Opera: [37, 0, 0],
    Safari: [12, 0, 0],
  },
  RegexpUnicodePropertyEscapes: {
    // Note: The latest version of "Chrome" failed this test: RegExp Unicode Property Escapes: Unicode 15.1
    // Note: The latest version of "Firefox" failed this test: RegExp Unicode Property Escapes: Unicode 15.1
    // Note: The latest version of "Hermes" failed 8 tests including: RegExp Unicode Property Escapes: Unicode 11
    // Note: The latest version of "IE" failed 8 tests including: RegExp Unicode Property Escapes: Unicode 11
    // Note: The latest version of "IOS" failed this test: RegExp Unicode Property Escapes: Unicode 15.1
    // Note: The latest version of "Rhino" failed 8 tests including: RegExp Unicode Property Escapes: Unicode 11
    // Note: The latest version of "Safari" failed this test: RegExp Unicode Property Escapes: Unicode 15.1
    ES: [2018, 0, 0],
    Node: [21, 3, 0],
  },
  RestArgument: {
    // Note: The latest version of "Hermes" failed this test: rest parameters: function 'length' property
    // Note: The latest version of "IE" failed 5 tests including: rest parameters: arguments object interaction
    // Note: The latest version of "Rhino" failed 5 tests including: rest parameters: arguments object interaction
    Chrome: [47, 0, 0],
    Deno: [1, 0, 0],
    Edge: [12, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [43, 0, 0],
    IOS: [10, 0, 0],
    Node: [6, 0, 0],
    Opera: [34, 0, 0],
    Safari: [10, 0, 0],
  },
  TemplateLiteral: {
    // Note: The latest version of "Hermes" failed this test: template literals: TemplateStrings call site caching
    // Note: The latest version of "IE" failed 7 tests including: template literals: TemplateStrings call site caching
    // Note: The latest version of "Rhino" failed 2 tests including: template literals: basic functionality
    Chrome: [41, 0, 0],
    Deno: [1, 0, 0],
    Edge: [13, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [34, 0, 0],
    IOS: [13, 0, 0],
    Node: [10, 0, 0],
    Opera: [28, 0, 0],
    Safari: [13, 0, 0],
  },
  TopLevelAwait: {
    Chrome: [89, 0, 0],
    Deno: [1, 0, 0],
    Edge: [89, 0, 0],
    ES: [2022, 0, 0],
    Firefox: [89, 0, 0],
    IOS: [15, 0, 0],
    Node: [14, 8, 0],
    Opera: [75, 0, 0],
    Safari: [15, 0, 0],
  },
  TypeofExoticObjectIsObject: {
    Chrome: [0, 0, 0],
    Deno: [0, 0, 0],
    Edge: [0, 0, 0],
    ES: [2020, 0, 0],
    Firefox: [0, 0, 0],
    Hermes: [0, 0, 0],
    IOS: [0, 0, 0],
    Node: [0, 0, 0],
    Opera: [0, 0, 0],
    Rhino: [0, 0, 0],
    Safari: [0, 0, 0],
  },
  UnicodeEscapes: {
    // Note: The latest version of "IE" failed 2 tests including: Unicode code point escapes: in identifiers
    // Note: The latest version of "Rhino" failed 4 tests including: Unicode code point escapes: in identifiers
    Chrome: [44, 0, 0],
    Deno: [1, 0, 0],
    Edge: [12, 0, 0],
    ES: [2015, 0, 0],
    Firefox: [53, 0, 0],
    Hermes: [0, 7, 0],
    IOS: [9, 0, 0],
    Node: [4, 0, 0],
    Opera: [31, 0, 0],
    Safari: [9, 0, 0],
  },
  Using: {},
};

const getUnsupportedFeatures = (name: string, versionStr: string) => {
  const features = Object.keys(jsTable);
  const version = versionStr.split(".").slice(0, 3).map((v) => parseInt(v, 10));
  if (version.some((v) => isNaN(v))) {
    // invalid version
    return [];
  }
  if (version.length === 1) {
    version.push(0, 0);
  } else if (version.length === 2) {
    version.push(0);
  }
  return features.filter((feature) => {
    const v = jsTable[feature][name];
    if (!v) {
      return true;
    }
    return versionLargeThan(v, version as Version);
  });
};

const versionLargeThan = (v1: Version, v2: Version) => {
  return v1[0] > v2[0] ||
    (v1[0] === v2[0] && v1[1] > v2[1]) ||
    (v1[0] === v2[0] && v1[1] === v2[1] && v1[2] > v2[2]);
};

const getBrowserInfo = (ua: string): { name?: string; version?: string } => {
  const info = uaParser(ua).browser;
  if (info.name === "HeadlessChrome") {
    info.name = "Chrome";
  } else if (info.name === "Safari" && ua.includes("iPhone;")) {
    info.name = "iOS";
  }
  return uaParser(ua).browser;
  return info;
};

const esmaUnsupportedFeatures: [string, number][] = [
  "es2022",
  "es2021",
  "es2020",
  "es2019",
  "es2018",
  "es2017",
  "es2016",
  "es2015",
].map((esma) => [
  esma,
  getUnsupportedFeatures(esma.slice(0, 2).toUpperCase(), esma.slice(2)).length,
]);

const rVersion = /^(\d+)\.(\d+)\.(\d+)/;

/** get build target from the `User-Agent` header by checking the `jsTable` object. */
export const getBuildTargetFromUA = (userAgent: string | null) => {
  if (!userAgent || userAgent.startsWith("curl/")) {
    return "esnext";
  }
  if (userAgent.startsWith("Deno/")) {
    const v = userAgent.slice(5).match(rVersion);
    if (v) {
      const version = v.slice(1).map((v) => parseInt(v, 10)) as Version;
      if (!versionLargeThan(version, [1, 33, 1])) {
        return "deno";
      }
    }
    return "denonext";
  }
  if (
    userAgent === "undici" ||
    userAgent.startsWith("Node/") ||
    userAgent.startsWith("Bun/")
  ) {
    return "node";
  }
  const browser = getBrowserInfo(userAgent);
  if (!browser.name || !browser.version) {
    return "esnext";
  }
  const unsupportFeatures = getUnsupportedFeatures(
    browser.name,
    browser.version,
  );
  for (const [esma, n] of esmaUnsupportedFeatures) {
    if (unsupportFeatures.length <= n) {
      return esma;
    }
  }
  return "esnext";
};
