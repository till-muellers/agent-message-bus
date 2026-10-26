/**
 * The bus protocol version this extension needs: the server's `Protocol` constant
 * (`server/internal/server/protocol.go`) is the source of truth, and `tests/version/protocol-sync.test.ts`
 * fails when the two disagree (`docs/design/arch/pipeline.md` §3.3).
 */

/** The oldest server this plugin works with. */
export const MIN_SERVER_PROTOCOL = 2;

/** The refusal text when the server is too old; undefined when it may be used. */
export function tooOld(found: number | undefined, url: string): string | undefined {
	if (found !== undefined && found >= MIN_SERVER_PROTOCOL) return undefined;
	return `the server at ${url} is too old (protocol ${found ?? "none"}, this plugin needs ${MIN_SERVER_PROTOCOL}); upgrade the agent-message-bus server`;
}
