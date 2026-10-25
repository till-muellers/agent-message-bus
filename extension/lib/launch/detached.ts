/** Starts a process that outlives this one and is never awaited. */
import { spawn } from "node:child_process";

export type DetachedSpawner = (command: string, args: string[]) => Promise<void>;

export const spawnDetached: DetachedSpawner = (command, args) => {
	const { promise, resolve, reject } = Promise.withResolvers<void>();
	const child = spawn(command, args, { detached: true, stdio: "ignore" });
	child.once("spawn", () => {
		child.unref();
		resolve();
	});
	child.once("error", (error) => reject(new Error(`run ${command}: ${error.message}`, { cause: error })));
	return promise;
};
