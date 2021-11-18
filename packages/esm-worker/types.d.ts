type Worker = {
	(req: Request): Response | Promise<Response>
}

type AppFileSystem = {
	readFile(): Promise<Uint8Array>
}


type KVPutOptions = {
	expiration?: number,
	expirationTtl?: number,
	metadata?: any
}

type AppStorage = {
	get(key: string, options?: { cacheTtl?: number }): Promise<string | null>
	get(key: string, options: { type: 'text', cacheTtl?: number }): Promise<string | null>
	get<T = any>(key: string, options: { type: 'json', cacheTtl?: number }): Promise<T | null>
	get(key: string, options: { type: 'arrayBuffer', cacheTtl?: number }): Promise<ArrayBuffer | null>
	get(key: string, options: { type: 'stream', cacheTtl?: number }): Promise<ReadableStream | null>
	put(key: string, value: string | ArrayBuffer | ArrayBuffer, options?: KVPutOptions): Promise<void>
	delete(key: string): Promise<void>
}

export type Options = {
	appFileSystem: AppFileSystem,
	appStorage: AppStorage,
	compileWorker: Worker,
	loadWorker: Worker,
	ssrWorker: Worker,
	isDev: boolean
}

export const createESMWorker: (options: Options) => Worker