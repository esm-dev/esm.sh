// https://nodejs.org/api/readline.html
class Interface extends A {
    get line() { return '' }
    get cursor() { return 0 }
    close() {}
    pause() {}
    prompt() {}
    question() {}
    resume() {}
    setPrompt() {}
    getPrompt() {}
    write() {}
    getCursorPos() { return {rows: 0, cols: 0} }
}

function clearLine() {}
function clearScreenDown() {}
function createInterface() { return new Interface() }
function cursorTo() {}
function emitKeypressEvents() {}
function moveCursor() {}

export {
    Interface,
    clearLine,
    clearScreenDown,
    createInterface,
    cursorTo,
    emitKeypressEvents,
    moveCursor,
}

export default {
    Interface,
    clearLine,
    clearScreenDown,
    createInterface,
    cursorTo,
    emitKeypressEvents,
    moveCursor,
}
