type Worker = {
	(req: Request): Response
}

interface AppFileSystem {
	readFile(): Promise<Uint8Array>
}

interface AppStorage { }

export type Options = {
	appFileSystem: AppFileSystem,
	appStorage: AppStorage,
	compileWorker: Worker,
	loadWorker: Worker,
	ssrWorker: Worker,
	isDev: boolean
}

export const createESMWorker: (options: Options) => void