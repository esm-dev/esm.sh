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
			onReady(p)
			break
		}
	}
	await p.status()
	p.close()
}

startServer(async (pp) => {
	const p = Deno.run({
		cmd: [Deno.execPath(), 'test', '-A', '--unstable', '--location=http://0.0.0.0/'],
		stdout: 'inherit',
		stderr: 'inherit'
	})
	await p.status()
	p.close()
	Promise.resolve().then(()=>{
		pp.kill(Deno.Signal.SIGKILL)
	})
})
