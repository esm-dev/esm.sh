# esm-compat

Check the ES module compatibility of a browser.

## Installation

```
npm i esm-compat
```

## Usage

```js
import { getBuildTargetFromUA } from 'esm-compat';

const ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"
const target = getBuildTargetFromUA(ua)
console.log(target) // => es2022
```

## API

```ts
export const targets: Set<string>;
export const getBuildTargetFromUA: (ua: string | null) => string;
```

## License

MIT
