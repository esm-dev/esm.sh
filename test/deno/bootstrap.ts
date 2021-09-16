import { existsSync } from 'https://deno.land/std@0.106.0/fs/exists.ts'

async function startServer(onReady: (p: any) => void) {
	const p = Deno.run({
		cmd: ['go', 'run', 'main.go', '-dev', '-port', '8080'],
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
		if (output.includes('cjs lexer server ready')) {
			Promise.resolve().then(() => onReady(p))
			break
		}
	}
	await p.status()
	p.close()
}

startServer(async (pp) => {
 	await test('test/deno/common/')
 	await test('test/deno/react/')
 	await test('test/deno/preact/')
	pp.kill('SIGTERM')
})

async function test(dir: string) {
	const cmd = [Deno.execPath(), 'test', '-A', '--unstable', '--location=http://0.0.0.0/']
	if (existsSync(dir + 'tsconfig.json')) {
		cmd.push('--config', dir + 'tsconfig.json')
	}
	cmd.push(dir)
	const p = Deno.run({
		cmd,
		stdout: 'inherit',
		stderr: 'inherit'
	})
	await p.status()
	p.close()
}
