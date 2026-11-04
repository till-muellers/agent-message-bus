import { takeField } from "../view/form-field.ts";
import { h, type Handler, type VNode } from "../view/h.ts";

function submitToken(signIn: (token: string) => void): Handler {
	return (event) => {
		event.preventDefault();
		const token = takeField(event, "token");
		if (token !== "") signIn(token);
	};
}

/** The token form; error is the server's message after a refused sign-in. */
export function loginView(error: string | undefined, signIn: (token: string) => void): VNode {
	return h(
		"main",
		{ key: "login" },
		h("h1", null, "agent-message-bus"),
		h(
			"form",
			{ on: { submit: submitToken(signIn) } },
			h("label", { for: "token" }, "Admin or agent token"),
			h("input", { id: "token", name: "token", type: "password", autocomplete: "off", required: true }),
			h("button", { type: "submit" }, "Sign in"),
		),
		error === undefined ? null : h("p", { role: "alert" }, error),
	);
}
