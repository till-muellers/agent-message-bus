/** A failed call: the HTTP status (0 when no response arrived) and the server's error code and message. */
export class ApiError extends Error {
	constructor(
		readonly status: number,
		readonly code: string,
		message: string,
	) {
		super(message);
		this.name = "ApiError";
	}
}

/** The message to show for any thrown value. */
export function errorMessage(err: unknown): string {
	return err instanceof Error ? err.message : String(err);
}

/** Any thrown value as an ApiError; anything else is a client-side failure (status 0). */
export function toApiError(err: unknown): ApiError {
	return err instanceof ApiError ? err : new ApiError(0, "client_error", errorMessage(err));
}
