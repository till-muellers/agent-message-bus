import { describe, expect, test } from "bun:test";
import { attribute, ATTRIBUTED_TOOLS } from "../../extension/lib/tools/attribution.ts";

describe("attribute", () => {
	test("the body of a post gets the attribution line of the subagent in front", () => {
		expect(attribute({ body: "tests pass", space: "demo", threadId: 7 }, "0-Explore")).toEqual({
			body: "[via subagent 0-Explore]\ntests pass",
			space: "demo",
			threadId: 7,
		});
	});

	test("the other arguments stay as they are", () => {
		expect(attribute({ blackboard: "plan", title: "draft", body: "plan draft" }, "Main.Child")).toEqual({
			blackboard: "plan",
			title: "draft",
			body: "[via subagent Main.Child]\nplan draft",
		});
	});

	test("a call without a body stays as it is", () => {
		expect(attribute({ space: "demo" }, "0-Explore")).toEqual({ space: "demo" });
	});

	test("the attributed tools are the ones that write a body", () => {
		expect(ATTRIBUTED_TOOLS).toEqual(["bus_add_post", "bus_create_thread"]);
	});
});
