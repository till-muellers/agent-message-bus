/** What the omp context says about the current session; updated on every event and command. */
export interface SessionView {
	mode?: string;
	sub: boolean;
	/** The id of the agent that runs the session (`ctx.agent.id`); the subagent's id in a subagent session. */
	agentId?: string;
	/** The session's file (`ctx.sessionManager.getSessionFile()`): the root key of a main session, the way to it for a subagent. */
	sessionFile?: string;
	/** Whether the agent is idle right now (bound to the latest omp context). */
	isIdle(): boolean;
	/** Sets or (with `undefined`) removes a status line entry (bound to the latest omp context). */
	setStatus(key: string, text: string | undefined): void;
}
