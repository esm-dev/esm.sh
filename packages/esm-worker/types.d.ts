export type Worker = {
	(req: Request): Response | Promise<Response>
}

export type DirEntry = {
	name: string
	isDir: boolean
}

export type AppFileSystem = {
	readDir(path: string | URL): AsyncIterable<DirEntry>
	readFile(path: string | URL): Promise<Uint8Array>
	readTextFile(path: string | URL): Promise<String>
}

type KVPutOptions = {
	expiration?: number,
	expirationTtl?: number,
	metadata?: any
}

export type AppStorage = {
	get(key: string, options?: { cacheTtl?: number }): Promise<string | null>
	get(key: string, options: { type: 'text', cacheTtl?: number }): Promise<string | null>
	get<T = any>(key: string, options: { type: 'json', cacheTtl?: number }): Promise<T | null>
	get(key: string, options: { type: 'arrayBuffer', cacheTtl?: number }): Promise<ArrayBuffer | null>
	get(key: string, options: { type: 'stream', cacheTtl?: number }): Promise<ReadableStream | null>
	put(key: string, value: string | ArrayBuffer | ReadableStream, options?: KVPutOptions): Promise<void>
	delete(key: string): Promise<void>
}

export type Options = {
	appFileSystem: AppFileSystem,
	appStorage: AppStorage,
	appWorker: Worker,
	appDevWorker?: Worker,
	compileWorker: Worker
	isDev?: boolean
}

export const createESMWorker: (options: Options) => Worker
