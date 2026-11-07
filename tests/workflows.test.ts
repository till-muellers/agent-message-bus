/**
 * Workflow policy fitness (`docs/design/arch/pipeline.md` §2, `.omp/AGENTS.md` "Git Flow"): CI costs money, so the
 * cheap tier (`ci-*.yml`) is path-filtered and superseded runs are cancelled; every job of every workflow runs on
 * `ubuntu-latest` (Windows and macOS are covered by local runs); the full tier (`full.yml`) runs
 * only for merges to `main` and `v*` tags; `release.yml` only publishes; no third-party action beyond the known ones.
 */
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";

const WORKFLOWS = resolve(import.meta.dir, "..", ".github", "workflows");
/** Branches the cheap tier runs on (`.omp/AGENTS.md` "Git Flow"). */
const CHEAP_BRANCHES = ["develop", "feature/**", "release/**", "hotfix/**"];
/** Actions already in use: GitHub's own, bun's setup and the Go linter's. Anything else needs approval first. */
const ALLOWED_ACTIONS = [/^actions\/[\w-]+@v\d+$/, /^oven-sh\/setup-bun@v\d+$/, /^golangci\/golangci-lint-action@v\d+$/];

type Json = Record<string, unknown>;

interface Workflow {
	file: string;
	on: Json;
	concurrency: Json | undefined;
	jobs: Record<string, Json>;
	text: string;
}

function asRecord(value: unknown): Json {
	return typeof value === "object" && value !== null && !Array.isArray(value) ? (value as Json) : {};
}

function loadWorkflow(file: string): Workflow {
	const text = readFileSync(join(WORKFLOWS, file), "utf8");
	const doc = asRecord(Bun.YAML.parse(text));
	return {
		file,
		on: asRecord(doc.on),
		concurrency: doc.concurrency === undefined ? undefined : asRecord(doc.concurrency),
		jobs: Object.fromEntries(Object.entries(asRecord(doc.jobs)).map(([name, job]) => [name, asRecord(job)])),
		text,
	};
}

const files = readdirSync(WORKFLOWS).filter(file => /\.ya?ml$/.test(file));
const cheap = files.filter(file => file.startsWith("ci-")).map(loadWorkflow);
const all = files.map(loadWorkflow);

function stepsOf(job: Json): Json[] {
	return Array.isArray(job.steps) ? job.steps.map(asRecord) : [];
}

describe("the workflow files", () => {
	test("are the cheap tier (ci-*.yml), the full tier, the publisher or a manual-*.yml", () => {
		const unknown = files.filter(file => !file.startsWith("ci-") && !file.startsWith("manual-") && file !== "full.yml" && file !== "release.yml");
		expect(unknown).toEqual([]);
		expect(files).toContain("full.yml");
		expect(files).toContain("release.yml");
	});

	test("the cheap tier has one workflow per area", () => {
		expect(cheap.map(workflow => workflow.file).sort()).toEqual(["ci-go.yml", "ci-python.yml", "ci-typescript.yml", "ci-web.yml"]);
	});

	test("use only the actions already in use, pinned to a major version", () => {
		const offenders = all.flatMap(workflow =>
			[...workflow.text.matchAll(/^\s*-?\s*uses:\s*(\S+)/gm)]
				.map(match => match[1] ?? "")
				.filter(action => !ALLOWED_ACTIONS.some(allowed => allowed.test(action)))
				.map(action => `${workflow.file}: ${action}`),
		);
		expect(offenders).toEqual([]);
	});

	test("run every job on ubuntu-latest, never on Windows or macOS", () => {
		const offenders = all.flatMap(workflow =>
			Object.entries(workflow.jobs)
				.filter(([, job]) => job["runs-on"] !== "ubuntu-latest")
				.map(([name]) => `${workflow.file}: ${name}`),
		);
		expect(offenders).toEqual([]);
		expect(all.flatMap(workflow => Object.values(workflow.jobs)).every(job => job.strategy === undefined)).toBe(true);
	});

	test("do not chain on each other with workflow_run", () => {
		expect(all.filter(workflow => "workflow_run" in workflow.on).map(workflow => workflow.file)).toEqual([]);
	});
});

describe.each(cheap.map(workflow => [workflow.file, workflow] as const))("cheap tier %s", (_file, workflow) => {
	test("runs on pushes to the Git Flow branches and on pull requests, never on main or tags", () => {
		const push = asRecord(workflow.on.push);
		expect(push.branches).toEqual(CHEAP_BRANCHES);
		expect(push.tags).toBeUndefined();
		expect(workflow.on).toHaveProperty("pull_request");
		expect(Object.keys(workflow.on).sort()).toEqual(["pull_request", "push"]);
	});

	test("is path-filtered, the same for push and pull_request, and counts its own workflow file", () => {
		const push = asRecord(workflow.on.push).paths;
		const pullRequest = asRecord(workflow.on.pull_request).paths;
		expect(Array.isArray(push) && push.length > 0).toBe(true);
		expect(pullRequest).toEqual(push);
		const own = `.github/workflows/${workflow.file}`;
		const paths = push as string[];
		expect(paths.includes(own) || paths.includes(".github/workflows/**")).toBe(true);
	});

	test("cancels superseded runs of the same branch", () => {
		expect(workflow.concurrency?.["cancel-in-progress"]).toBe(true);
		expect(workflow.concurrency?.group).toBe("${{ github.workflow }}-${{ github.head_ref || github.ref_name }}");
	});

	test("runs without a matrix and without the race detector", () => {
		for (const job of Object.values(workflow.jobs)) {
			expect(job.strategy).toBeUndefined();
			expect(stepsOf(job).some(step => String(step.run ?? "").includes("-race"))).toBe(false);
		}
	});
});

describe("full tier full.yml", () => {
	const full = loadWorkflow("full.yml");

	test("runs on main, v* tags and by hand, never for pull requests, develop or feature branches", () => {
		const push = asRecord(full.on.push);
		expect(push.branches).toEqual(["main"]);
		expect(push.tags).toEqual(["v*"]);
		expect(full.on).toHaveProperty("workflow_dispatch");
		expect(Object.keys(full.on).sort()).toEqual(["push", "workflow_dispatch"]);
		expect(JSON.stringify(full.on)).not.toMatch(/develop|feature|hotfix|release\//);
	});

	test("cancels superseded runs of the same ref", () => {
		expect(full.concurrency?.["cancel-in-progress"]).toBe(true);
		expect(full.concurrency?.group).toBe("${{ github.workflow }}-${{ github.event_name }}-${{ github.ref }}");
	});

	test("runs the Go tests with the race detector", () => {
		const go = full.jobs.go as Json;
		expect(stepsOf(go).some(step => String(step.run).includes("go test -race"))).toBe(true);
	});

	test("covers every area of the cheap tier and the packaging build", () => {
		expect(Object.keys(full.jobs)).toEqual(expect.arrayContaining(["go", "typescript", "web", "python", "package"]));
		expect(full.text).toContain("scripts/build.py");
	});

	test("skips the suites when the run of main already covered the tagged commit", () => {
		for (const name of ["go", "typescript", "web", "python", "package"]) {
			expect(full.jobs[name]?.needs).toBe("covered");
			expect(full.jobs[name]?.if).toBe("needs.covered.outputs.covered != 'true'");
		}
		const lookup = stepsOf(full.jobs.covered as Json).map(step => String(step.run ?? "")).join("\n");
		expect(lookup).toContain('"$GITHUB_REF_TYPE" = tag');
		expect(lookup).toContain("workflows/full.yml/runs");
		expect(lookup).toContain("release_ci_gate.py");
	});
});

describe("publisher release.yml", () => {
	const release = loadWorkflow("release.yml");

	test("runs only for v* tags", () => {
		expect(release.on).toEqual({ push: { tags: ["v*"] } });
	});

	test("checks the tag, then the full-tier run of the commit, and runs no test suite", () => {
		const runs = Object.values(release.jobs).flatMap(stepsOf).map(step => String(step.run ?? ""));
		const version = runs.findIndex(run => run.includes("release_version.py"));
		const gate = runs.findIndex(run => run.includes("workflows/full.yml/runs") && run.includes("release_ci_gate.py"));
		expect(version).toBeGreaterThanOrEqual(0);
		expect(gate).toBeGreaterThan(version);
		expect(runs.some(run => /go test|bun test|pytest/.test(run))).toBe(false);
	});

	test("marks a pre-release tag as a GitHub pre-release, so it never becomes the latest release", () => {
		expect(release.text).toContain("--prerelease");
	});
});
