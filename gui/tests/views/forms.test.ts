import { describe, expect, test } from "bun:test";
import type { VNode } from "../../src/view/h.ts";
import { inviteForm, postForm, sendForm } from "../../src/views/forms.ts";
import { byTag, findAll, textOf } from "../tree.ts";

const none = { invite: () => {}, post: () => {}, send: () => {} };

function fieldsOf(form: VNode): (string | undefined)[] {
	return findAll(form, (n) => n.tag === "input" || n.tag === "textarea" || n.tag === "select").map((n) => n.attrs.name);
}

describe("the forms", () => {
	test("the invite form asks for a name and an optional task", () => {
		const form = inviteForm("demo", none);
		expect(fieldsOf(form)).toEqual(["name", "task"]);
		expect(typeof form.on.submit).toBe("function");
	});

	test("the send form offers the topics in the server's order and a critical flag", () => {
		const form = sendForm("demo", ["all", "notes"], none);
		expect(fieldsOf(form)).toEqual(["topic", "body", "critical"]);
		expect(findAll(form, byTag("option")).map(textOf)).toEqual(["all", "notes"]);
	});

	test("the post form asks for a body", () => {
		const form = postForm("demo", 1, none);
		expect(fieldsOf(form)).toEqual(["body"]);
		expect(textOf(form)).toContain("Post reply");
	});
});
