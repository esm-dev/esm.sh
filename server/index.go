package server

const indexHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charSet="utf-8" />
    <meta name="viewport" content="user-scalable=no,initial-scale=1.0,minimum-scale=1.0,maximum-scale=1.0,minimal-ui" />
    <title>ESM</title>
</head>
<body>
    <h1>ESM</h1>
</body>
</html>
`

const bundleHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charSet="utf-8" />
    <meta name="viewport" content="user-scalable=no,initial-scale=1.0,minimum-scale=1.0,maximum-scale=1.0,minimal-ui" />
    <title>ESM Bundler</title>
</head>
<body>
    <main><em>Loading...</em></main>
    <script type="module">
        import React from '/[react,react-dom]/react'
        import ReactDom from '/[react,react-dom]/react-dom'

        ReactDom.render(
            React.createElement('h1', null, 'ESM Bundler [WIP]'),
            document.querySelector('main')
        )
    </script>
</body>
</html>
`
