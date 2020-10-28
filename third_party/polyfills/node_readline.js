// https://nodejs.org/api/readline.html

class Interface {
    close() {}
    pause() {}
    prompt() {}
    question() {}
    resume() {}
    setPrompt() {}
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
