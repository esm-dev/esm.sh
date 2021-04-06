export default [
  /* UI Library */
  {
    name: 'antd@4',
    namedExport: ['Button', 'Card'],
  },
  {
    name: '@ant-design/icons@4',
    namedExport: ['ArrowLeftOutlined'],
  },
  {
    name: '@formily/antd@1',
    namedExport: ['Field', 'Form'],
  },
  {
    name: '@alist/antd@1',
    namedExport: ['List', 'Table'],
  },
  {
    name: '@alifd/next@1',
    namedExport: ['Button', 'Card'],
  },
  {
    name: '@formily/next@1',
    namedExport: ['Field', 'Form'],
  },
  {
    name: '@alist/next@1',
    namedExport: ['List', 'Table'],
  },

  /* Tools */
  {
    name: 'qs',
    namedExport: ['parse', 'stringify'],
  },
  {
    name: 'jsonpath@1',
    default: ['parse', 'stringify'],
  },
  {
    name: 'is@3',
    namedExport: ['number', 'arguments'],
    default: ['number', 'arguments'],
  },
  {
    name: 'koa-compose@4',
    defaultIs: 'function',
  },
  {
    name: 'domhandler@4',
    namedExport: ['DomHandler'],
    defaultIs: 'function',
  },
  {
    name: 'react-eva@1',
    namedExport: ['connect'],
  },
  {
    name: 'react-refresh-typescript',
    defaultIs: 'function',
  },
  {
    name: 'bizcharts@4',
    namedExport: ['Area'],
  },
  {
    name: 'react-router-dom@5',
    namedExport: ['Link', 'HashRouter'],
  },
  {
    name: 'rxjs@6',
    namedExport: ['VirtualAction', 'fromEventPattern'],
  },
  {
    name: '@emotion/styled@10',
    default: ['a', 'abbr'],
    defaultIs: 'function',
  },
  {
    name: 'debug@4',
    default: ['colors', 'debug'],
  },
  {
    name: 'ali-oss@6',
    defaultIs: 'function',
    default: ['Buffer'],
  },
  {
    name: 'underscore@1',
    defaultIs: 'function',
    default: ['after', 'all'],
  },
  {
    name: 'fn-args@4',
    defaultIs: 'function',
  },
  {
    name: 'moment@2',
    defaultIs: 'function',
    default: ['isDate', 'isMoment'],
  },
  {
    name: 'sort-object@3',
    defaultIs: 'function',
  },
  {
    name: 'react-copy-to-clipboard@5',
    defaultIs: 'function',
  },
  {
    name: 'uuid@8',
    namedExport: ['v1', 'parse', 'stringify'],
  },
  {
    name: 'generate-schema@2',
    default: ['json', 'mysql'],
  },
  {
    name: 'async',
    namedExport: ['all', 'any'],
    default: ['all', 'any'],
  },
  {
    name: 'request',
    defaultIs: 'function',
    default: ['Request', 'cookie'],
  },
  {
    name: 'jsdom',
    namedExport: ['JSDOM', 'CookieJar'],
  },
  {
    name: 'mocha@8',
    default: ['Mocha'],
  },
  {
    name: 'chalk',
  },
  {
    name: 'axios',
  },
  {
    name: 'core-js',
  },
  {
    name: 'bluebird',
  },
  {
    name: 'classnames',
  },
  {
    name: 'yargs',
  },
  {
    name: 'glob',
  },
  {
    name: 'colors',
  },
  {
    name: 'minimist',
  },
  {
    name: 'semver',
  },
  {
    name: 'redux',
  },
  {
    name: 'babel-runtime/core-js/set',
    default: ['from', 'of'],
  },
  {
    name: '@babel/core',
  },
  {
    name: 'styled-components',
  },
  {
    name: 'react-redux',
  },
  {
    name: 'cheerio',
  },
  {
    name: 'vue-router',
  },
  {
    name: 'ramda',
  },
  {
    name: 'q',
  },
  {
    name: 'handlebars',
  },
  {
    name: 'art-template@4',
    default: ['compile', 'render'],
  },
  {
    name: 'domhandler',
  },
  {
    name: 'htmlparser2',
  },
  {
    name: 'dom-serializer',
  },
  {
    name: 'fetch',
    namedExport: ['fetchUrl'],
  },
  {
    name: 'd3',
  },
  {
    name: 'chalk',
    defaultIs: 'function',
  },
  {
    name: 'ajax',
    namedExport: ['getJSON'],
  },
  {
    name: 'semver',
    namedExport: ['eq', 'diff'],
  },
  {
    name: 'co',
  },
  {
    name: 'cheerio',
    defaultIs: 'function',
  },
  {
    name: 'async',
  },
  {
    name: 'prop-types',
  },
  {
    name: 'xss',
  },
  {
    name: 'lume',
  },
  {
    name: '@lume/element',
    namedExport: ['Element', 'reactive'],
  },
  {
    name: '@lume/variable',
    namedExport: ['variable', 'autorun'],
  },
  {
    name: 'qiankun@2',
    namedExport: ['registerMicroApps', 'start'],
  },
  {
    name: 'single-spa@5',
    namedExport: ['start', 'getAppNames'],
  },
]

export const assert = {
  hasNamedExport: (mod, name) => {
    if (mod[name] === undefined) throw new Error(`[namedExport] ${name} is not exist`);
  },
  hasDefault: (mod, name) => {
    if (mod[name] === undefined) throw new Error(`[default] ${name} is not exist`);
  },
}
