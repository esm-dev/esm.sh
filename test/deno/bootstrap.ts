import { existsSync } from 'https://deno.land/std@0.119.0/fs/exists.ts'

const [select] = Deno.args
if (select) {
	await test(`test/deno/${select}/`)
	Deno.exit(0)
} else {
	startServer(async (p) => {
		try {
			await test('test/deno/common/', p)
			await test('test/deno/preact/', p)
			await test('test/deno/prismjs/', p)
			await test('test/deno/react/', p)
			console.log('Done')
		} catch (error) {
			console.error(error)
		}
		p.kill('SIGINT')
	})
}

async function startServer(onReady: (p: any) => void) {
	await run('go', 'build', '-o', 'esmd', 'main.go')
	const p = Deno.run({
		cmd: ['./esmd', '-dev', '-port', '8080'],
		stdout: 'piped',
		stderr: 'inherit'
	})
	let output = ''
	const buf = new Uint8Array(32)
	for (let index = 0; index < 1000; index++) {
		const n = await p.stdout?.read(buf)
		if (!n) {
			break
		}
		output += new TextDecoder().decode(buf.slice(0, n))
		if (output.includes('node services process started')) {
			onReady(p)
			break
		}
	}
	await p.status()
}

async function test(dir: string, p?: any) {
	const cmd = [Deno.execPath(), 'test', '-A', '--unstable', '-r', '--location=http://0.0.0.0/']
	if (existsSync(dir + 'tsconfig.json')) {
		cmd.push('--config', dir + 'tsconfig.json')
	}
	cmd.push(dir)
	const { code, success } = await run(...cmd)
	if (!success) {
		p?.kill('SIGINT')
		Deno.exit(code)
	}
}

async function run(...cmd: string[]) {
	return await Deno.run({ cmd, stdout: 'inherit', stderr: 'inherit' }).status()
}
