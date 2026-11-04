import { describe, expect, test } from "bun:test";
import { loginView } from "../../src/views/login.ts";
import { byTag, findAll, textOf } from "../tree.ts";

describe("the login view", () => {
	test("asks for a token in a password field that is never prefilled", () => {
		const [input] = findAll(loginView(undefined, () => {}), byTag("input"));
		expect(input?.attrs).toMatchObject({ name: "token", type: "password", autocomplete: "off" });
		expect(input?.attrs.value).toBeUndefined();
	});

	test("submits through a handler, not through a form action", () => {
		const [form] = findAll(loginView(undefined, () => {}), byTag("form"));
		expect(typeof form?.on.submit).toBe("function");
		expect(form?.attrs.action).toBeUndefined();
	});

	test("shows the server's message after a refused sign-in", () => {
		const view = loginView("too many failed sign-ins; try again later", () => {});
		const [alert] = findAll(view, (n) => n.attrs.role === "alert");
		expect(alert && textOf(alert)).toBe("too many failed sign-ins; try again later");
	});

	test("shows no message before the first attempt", () => {
		expect(findAll(loginView(undefined, () => {}), (n) => n.attrs.role === "alert")).toEqual([]);
	});
});
