import { version } from './version.ts'
import log from './log.ts'

const commands = ['add', 'remove', 'update', 'upgrade']
const helpMessage = `esmm v${version}
A fast, global content delivery network and package manager for ES Modules.

Docs: https://docs.esm.sh/
Bugs: https://github.com/postui/esmm/issues

Usage:
    esmm <command> [...options]

Commands:
    ${commands.join(', ')}

Options:
    -h, --help     Prints help message
    -v, --version  Prints version number
`

function main() {
    // parse deno args
    const args: Array<string> = []
    const argOptions: Record<string, string | boolean> = {}
    for (let i = 0; i < Deno.args.length; i++) {
        const arg = Deno.args[i]
        if (arg.startsWith('-')) {
            if (arg.includes('=')) {
                const [key, value] = arg.replace(/^-+/, '').split('=', 2)
                argOptions[key] = value
            } else {
                const key = arg.replace(/^-+/, '')
                const nextArg = Deno.args[i + 1]
                if (nextArg && !nextArg.startsWith('-')) {
                    argOptions[key] = nextArg
                    i++
                } else {
                    argOptions[key] = true
                }
            }
        } else {
            args.push(arg)
        }
    }

    // prints esmm version
    if (argOptions.v) {
        console.log(`esmm v${version}`)
        Deno.exit(0)
    }

    // prints esmm and deno version
    if (argOptions.version) {
        const { deno, v8, typescript } = Deno.version
        console.log(`esmm v${version}\ndeno v${deno}\nv8 v${v8}\ntypescript v${typescript}`)
        Deno.exit(0)
    }

    // prints help message
    const hasCommand = args.length > 0 && commands.includes(args[0])
    if (argOptions.h || argOptions.help) {
        if (hasCommand) {
            import(`./cli/${args.shift()}.ts`).then(({ helpMessage }) => {
                console.log(`esmm v${version}`)
                if (typeof helpMessage === 'string') {
                    console.log(helpMessage)
                }
                Deno.exit(0)
            })
            return
        } else {
            console.log(helpMessage)
            Deno.exit(0)
        }
    }

    // sets log level
    if (argOptions.l || argOptions.log) {
        log.setLevel(String(argOptions.l || argOptions.log))
    } 

    // execute command
    const command = hasCommand ? args.shift() : 'dev'
    import(`./cli/${command}.ts`).then(({ default: cmd }) => cmd(argOptions))
}

if (import.meta.main) {
    main()
}
