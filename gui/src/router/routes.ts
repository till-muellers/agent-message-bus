import type { Route } from "../state/app-state.ts";

const NAME = /^[a-z0-9][a-z0-9_-]{0,62}$/;
const THREAD_ID = /^[1-9][0-9]{0,14}$/;
const NOT_FOUND: Route = { name: "not-found" };

function isName(segment: string | undefined): segment is string {
	return segment !== undefined && NAME.test(segment);
}

function segmentsOf(hash: string): string[] | undefined {
	if (hash === "" || hash === "#" || hash === "#/") return [];
	if (!hash.startsWith("#/")) return undefined;
	try {
		return hash.slice(2).split("/").map(decodeURIComponent);
	} catch {
		return undefined;
	}
}

function spaceRoute(space: string, rest: string[]): Route | undefined {
	const [kind, id, ...extra] = rest;
	if (kind === undefined) return { name: "space", space };
	if (id === undefined || extra.length > 0) return undefined;
	if (kind === "boards" && isName(id)) return { name: "board", space, board: id };
	if (kind === "threads" && THREAD_ID.test(id)) return { name: "thread", space, threadId: Number(id) };
	return undefined;
}

function match(segments: string[]): Route | undefined {
	const [first, second, ...rest] = segments;
	if (first === undefined) return { name: "spaces" };
	if (second === undefined && (first === "inbox" || first === "admins")) return { name: first };
	if (first === "spaces" && isName(second)) return spaceRoute(second, rest);
	return undefined;
}

/** Maps a location hash to a route; anything malformed or with an invalid name is not-found. */
export function parseRoute(hash: string): Route {
	const segments = segmentsOf(hash);
	return (segments && match(segments)) ?? NOT_FOUND;
}

export function hrefOf(route: Route): string {
	switch (route.name) {
		case "spaces":
			return "#/";
		case "space":
			return `#/spaces/${encodeURIComponent(route.space)}`;
		case "board":
			return `#/spaces/${encodeURIComponent(route.space)}/boards/${encodeURIComponent(route.board)}`;
		case "thread":
			return `#/spaces/${encodeURIComponent(route.space)}/threads/${route.threadId}`;
		case "inbox":
		case "admins":
		case "not-found":
			return `#/${route.name}`;
	}
}

export function sameRoute(a: Route, b: Route): boolean {
	return a.name === b.name && hrefOf(a) === hrefOf(b);
}
