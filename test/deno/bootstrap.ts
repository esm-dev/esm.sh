import { existsSync } from 'https://deno.land/std@0.106.0/fs/exists.ts'

async function startServer(onReady: (p: any) => void) {
	await run('go', 'build', 'main.go')
	const p = Deno.run({
		cmd: ['./main', '-dev', '-port', '8080'],
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

startServer(async (p) => {
	await test('test/deno/common/')
	await test('test/deno/preact/')
	await test('test/deno/prismjs/')
	await test('test/deno/react/')
	p.kill('SIGINT')
}).then(() => {
	console.log('Done')
}).finally(() => {
	Deno.removeSync('./main')
})

async function test(dir: string) {
	const cmd = [Deno.execPath(), 'test', '-A', '--unstable', '-r', '--location=http://0.0.0.0/']
	if (existsSync(dir + 'tsconfig.json')) {
		cmd.push('--config', dir + 'tsconfig.json')
	}
	cmd.push(dir)
	await run(...cmd)
}

async function run(...cmd: string[]) {
	await Deno.run({
		cmd,
		stdout: 'inherit',
		stderr: 'inherit'
	}).status()
}
