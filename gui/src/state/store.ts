import type { Action } from "./actions.ts";
import type { AppState } from "./app-state.ts";

type Listener = (state: AppState) => void;

export interface Store {
	getState(): AppState;
	dispatch(action: Action): void;
	subscribe(listener: Listener): () => void;
}

export function createStore(reduce: (state: AppState, action: Action) => AppState, initial: AppState): Store {
	let state = initial;
	const listeners = new Set<Listener>();
	return {
		getState: () => state,
		dispatch(action) {
			const next = reduce(state, action);
			if (next === state) return;
			state = next;
			for (const listener of listeners) listener(state);
		},
		subscribe(listener) {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
	};
}
