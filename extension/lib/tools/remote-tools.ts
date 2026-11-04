/** Mirrors the bus server's tool registry into the omp session. */
import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import type { BusClient, RemoteTool } from "../bus/client.ts";
import type { ActiveCredentials } from "../session/stored-credentials.ts";
import { errorMessage, NOT_LOGGED_IN } from "../bus/errors.ts";
import { toZod } from "../bus/schema.ts";
import { fixedSender, isSendTool, outgoingCard, sentMessage } from "../render/outgoing-card.ts";
import { receiptText } from "../render/send-receipt.ts";
import { THREAD_TOOL, threadView } from "../render/thread-view.ts";
import { toolResult } from "./tool-result.ts";
import { attribute, ATTRIBUTED_TOOLS } from "./attribution.ts";
import { changesMembership } from "./membership-refresh.ts";
import { presentationOf } from "./presentation.ts";
import { selectTools } from "./tool-selection.ts";
import { WAIT_TOOL, type WaitToolDeps, waitToolDefinition } from "./wait-tool.ts";

/** Per-session registration: tools are fetched and registered once; logout does not reset it. */
export interface RegistrationState {
	registered: boolean;
	/** Names of the tools registered so far, switched on or off by `restart()` to match the resolved credentials. */
	names: string[];
}

export interface RemoteToolsOptions {
	client: BusClient;
	/** The credentials active at call time; undefined after logout. */
	getActive: () => ActiveCredentials | undefined;
	state: RegistrationState;
	/** Reports a tool that could not be registered; the other tools still register. */
	warn: (message: string) => void;
	/** The kind of the session: a subagent gets the allow-list of `tool-selection.ts`. */
	kind: "main" | "sub";
	/** The id of the subagent that writes through this session; undefined in a main session. */
	agentId?: string;
	/** What the local `bus_wait` of a main session waits with; a subagent has none. */
	wait?: WaitToolDeps;
	/** Told after every successful `bus_read_thread`: the thread was read. */
	threadRead?: (space: string, threadId: number) => void;
	/** Awaited after every successful membership tool: the session's spaces changed. */
	membershipChanged?: () => Promise<void>;
	/** The bus agent the session acts as, for the outgoing card of a send; undefined while unknown. */
	self?: () => string | undefined;
}

/** A tool this session registers: its name and how to build its definition. */
interface Registration {
	name: string;
	define(): Parameters<ExtensionAPI["registerTool"]>[0];
}

/** The tools this session registers: the selected server tools and, in a main session, the local `bus_wait` on the server's schema. */
function registrations(pi: ExtensionAPI, tools: readonly RemoteTool[], options: RemoteToolsOptions): Registration[] {
	const { kind, wait } = options;
	const shadow = kind === "main" && wait ? tools.find(tool => tool.name === WAIT_TOOL) : undefined;
	const remote = selectTools(tools, kind).map(tool => ({ name: tool.name, define: () => toolDefinition(pi, tool, options) }));
	if (!shadow || !wait) return remote;
	return [...remote, { name: WAIT_TOOL, define: () => waitToolDefinition(pi, shadow, wait) }];
}

/** Calls the server tool with the current credentials; a subagent says which subagent wrote a post. */
async function callRemote(tool: RemoteTool, params: Record<string, unknown>, options: RemoteToolsOptions): Promise<unknown> {
	const { client, getActive, kind, agentId } = options;
	const current = getActive();
	if (!current) throw new Error(NOT_LOGGED_IN);
	const attributed = kind === "sub" && agentId && ATTRIBUTED_TOOLS.includes(tool.name) ? attribute(params, agentId) : params;
	const result = await client.callTool(current.url, current.token, tool.name, attributed);
	if (tool.name === THREAD_TOOL && typeof params.space === "string" && typeof params.threadId === "number") options.threadRead?.(params.space, params.threadId);
	if (changesMembership(tool.name)) await options.membershipChanged?.();
	return result;
}

type Rendering = Pick<Parameters<ExtensionAPI["registerTool"]>[0], "renderCall" | "renderResult">;

/** `bus_read_thread` renders its result as a thread; a send renders its call as an outgoing card. */
function rendering(pi: ExtensionAPI, tool: RemoteTool, self: RemoteToolsOptions["self"]): Rendering {
	const { name } = tool;
	if (name === THREAD_TOOL) return { renderResult: (result, { expanded }, theme) => threadView(pi.pi, result, expanded, theme) };
	if (!isSendTool(name)) return {};
	const sender = fixedSender(self ?? (() => undefined));
	return { renderCall: (args, { expanded, isPartial }, theme) => outgoingCard(pi.pi, sentMessage(name, args, sender(args, isPartial)), expanded, theme) };
}

function toolDefinition(pi: ExtensionAPI, tool: RemoteTool, options: RemoteToolsOptions): Parameters<ExtensionAPI["registerTool"]>[0] {
	return {
		name: tool.name,
		label: tool.name,
		description: tool.description,
		parameters: toZod(tool.inputSchema, pi.zod),
		...presentationOf(tool.name),
		...rendering(pi, tool, options.self),
		execute: (_toolCallId: string, params: Record<string, unknown>) => toolResult(() => callRemote(tool, params, options), value => receiptText(tool.name, value)),
	};
}

/** Registers the tools this session gets once per session. Returns the count registered by this call. */
export async function registerRemoteTools(pi: ExtensionAPI, options: RemoteToolsOptions): Promise<number> {
	const { client, getActive, state, warn, kind } = options;
	if (state.registered) return 0;
	const active = getActive();
	if (!active) return 0;
	const tools = await client.listTools(active.url, active.token);
	state.registered = true;
	for (const { name, define } of registrations(pi, tools, options)) {
		try {
			pi.registerTool(define());
			state.names.push(name);
		} catch (error) {
			warn(`tool ${name} not registered: ${errorMessage(error)}`);
		}
	}
	return state.names.length;
}
