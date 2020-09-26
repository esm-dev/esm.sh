package server

const indexHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charSet="utf-8" />
    <meta name="viewport" content="user-scalable=no,initial-scale=1.0,minimum-scale=1.0,maximum-scale=1.0,minimal-ui" />
    <title>ESM</title>
</head>
<body>
    <main><em>Loading...</em></main>
    <script type="module">
        import { useState, createElement as h } from '/react?dev'
        import { render } from '/react-dom?dev'
        // import { render, h } from '/preact?dev'
        // import { useState } from '/preact/hooks?dev'

        function App() {
            const [count, setCount] = useState(0)

            return h('div', null,
                h('h1', null, 'ESM'),
                h('p', null, 'A fast, global content delivery network and package manager for ES Modules.'),
                h('p', null,
                    h('button', { onClick: () => setCount(n => n-1) }, '-'),
                    ' ',
                    h('span', null, count),
                    ' ',
                    h('button', { onClick: () => setCount(n => n+1) }, '+')
                ),
                h('p', null,
                    h('a', { href: 'https://github.com/postui/esmm' }, 'Github')
                ),
            )
        }

        render(h(App), document.querySelector('main'))
    </script>
</body>
</html>
`
