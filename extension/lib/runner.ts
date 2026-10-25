/** Runs a process to completion; injectable so tests never spawn anything real. */
import { execFile } from "node:child_process";

export interface RunResult {
	code: number;
	stdout: string;
	stderr: string;
}

export type Runner = (command: string, args: string[]) => Promise<RunResult>;

/** The real runner: `execFile` without a shell; a non-zero exit resolves with its code instead of rejecting. */
export const spawnRunner: Runner = (command, args) => {
	const { promise, resolve, reject } = Promise.withResolvers<RunResult>();
	execFile(command, args, { windowsHide: true }, (error, stdout, stderr) => {
		if (error && typeof error.code !== "number") return reject(new Error(`run ${command}: ${error.message}`, { cause: error }));
		resolve({ code: error ? Number(error.code) : 0, stdout: String(stdout), stderr: String(stderr) });
	});
	return promise;
};
