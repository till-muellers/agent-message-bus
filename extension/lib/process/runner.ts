/** Runs a process to completion; injectable so tests never spawn anything real. */
import { type ExecException, execFile } from "node:child_process";

export interface RunResult {
	code: number;
	stdout: string;
	stderr: string;
}

export interface RunOptions {
	/** Kills the process after this many milliseconds. */
	timeoutMs?: number;
	/** Kills the process when it aborts. */
	signal?: AbortSignal;
}

export type Runner = (command: string, args: string[], options?: RunOptions) => Promise<RunResult>;

/** Why the run produced no exit code: aborted, timed out or not started; undefined when it exited. */
function failure(command: string, error: ExecException | null, { timeoutMs, signal }: RunOptions): Error | undefined {
	if (!error) return undefined;
	if (signal?.aborted) return new Error(`run ${command}: aborted`, { cause: error });
	if (error.killed && timeoutMs !== undefined) return new Error(`run ${command}: timed out after ${timeoutMs} ms`, { cause: error });
	if (typeof error.code !== "number") return new Error(`run ${command}: ${error.message}`, { cause: error });
	return undefined;
}

/** The real runner: `execFile` without a shell; a non-zero exit resolves with its code instead of rejecting. */
export const spawnRunner: Runner = (command, args, options = {}) => {
	const { promise, resolve, reject } = Promise.withResolvers<RunResult>();
	execFile(command, args, { windowsHide: true, timeout: options.timeoutMs, signal: options.signal }, (error, stdout, stderr) => {
		const failed = failure(command, error, options);
		if (failed) return reject(failed);
		resolve({ code: error ? Number(error.code) : 0, stdout: String(stdout), stderr: String(stderr) });
	});
	return promise;
};
