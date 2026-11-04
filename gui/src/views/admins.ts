import type { AdminInfo } from "../api/types.ts";
import { takeField } from "../view/form-field.ts";
import { type ChildArg, h, type Handler, type VNode } from "../view/h.ts";
import { type Row, table } from "./table.ts";

export interface AdminAreaIntents {
	mintAdmin(name: string): void;
	revokeAdmin(name: string): void;
}

/** The bootstrap admin is revoked by rotating the instance token on the host, not here. */
function revokeButton(admin: AdminInfo, intents: AdminAreaIntents): ChildArg {
	if (admin.bootstrap) return null;
	return h("button", { type: "button", key: `revoke-${admin.name}`, on: { click: () => intents.revokeAdmin(admin.name) } }, "Revoke");
}

function row(admin: AdminInfo, intents: AdminAreaIntents): Row {
	const cells = [admin.name, admin.bootstrap ? "yes" : "no", admin.tokens, admin.spaces.join(", "), revokeButton(admin, intents)];
	return { key: admin.name, cells };
}

function submitName(mint: (name: string) => void): Handler {
	return (event) => {
		event.preventDefault();
		const name = takeField(event, "name");
		if (name !== "") mint(name);
	};
}

function mintForm(intents: AdminAreaIntents): VNode {
	return h(
		"form",
		{ key: "mint", on: { submit: submitName((name) => intents.mintAdmin(name)) } },
		h("label", { for: "admin-name" }, "New admin name"),
		h("input", { id: "admin-name", name: "name", autocomplete: "off", required: true }),
		h("button", { type: "submit" }, "Mint admin"),
	);
}

/** Every admin of the instance, a revoke button per minted admin and the mint form. */
export function adminsView(admins: readonly AdminInfo[], intents: AdminAreaIntents): VNode {
	const rows = admins.map((admin) => row(admin, intents));
	return h("section", { key: "admins" }, h("h1", null, "Admins"), table(["Name", "Bootstrap", "Tokens", "Spaces", ""], rows), mintForm(intents));
}
