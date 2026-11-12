/**
 * Workflow policy fitness (`docs/design/arch/pipeline.md` §2, `.omp/AGENTS.md` "Git workflow and CI"): CI costs money, so
 * nothing runs on a branch push. A pull request into `develop`, `release/**` or `hotfix/**` runs the reduced suite
 * (`ci-*.yml`, path-filtered); a pull request into `main` runs the full suite (`full.yml`, never path-filtered, so its
 * jobs can be required checks); a `v*` tag only publishes (`release.yml`). Drafts are skipped, superseded runs are
 * cancelled, every job runs on `ubuntu-latest` (Windows and macOS are covered by local runs) and no third-party action
 * beyond the known ones is used.
 */
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";

const WORKFLOWS = resolve(import.meta.dir, "..", ".github", "workflows");
/** The base branches of the reduced suite (`.omp/AGENTS.md` "Git workflow and CI"). */
const REDUCED_BASES = ["develop", "release/**", "hotfix/**"];
const DRAFT_GUARD = "github.event.pull_request.draft != true";
const CONCURRENCY_GROUP = "${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}";
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
const reduced = files.filter(file => file.startsWith("ci-")).map(loadWorkflow);
const all = files.map(loadWorkflow);

function stepsOf(job: Json): Json[] {
	return Array.isArray(job.steps) ? job.steps.map(asRecord) : [];
}

function runsOf(workflow: Workflow): string[] {
	return Object.values(workflow.jobs)
		.flatMap(stepsOf)
		.map(step => String(step.run ?? ""))
		.filter(run => run !== "");
}

describe("the workflow files", () => {
	test("are the reduced suite (ci-*.yml), the full suite, the publisher or a manual-*.yml", () => {
		const unknown = files.filter(file => !file.startsWith("ci-") && !file.startsWith("manual-") && file !== "full.yml" && file !== "release.yml");
		expect(unknown).toEqual([]);
		expect(files).toContain("full.yml");
		expect(files).toContain("release.yml");
	});

	test("the reduced suite has one workflow per area", () => {
		expect(reduced.map(workflow => workflow.file).sort()).toEqual(["ci-go.yml", "ci-python.yml", "ci-typescript.yml", "ci-web.yml"]);
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

	test("run every job on ubuntu-latest, never on Windows or macOS, without a matrix", () => {
		const jobs = all.flatMap(workflow => Object.entries(workflow.jobs).map(([name, job]) => ({ at: `${workflow.file}: ${name}`, job })));
		expect(jobs.filter(({ job }) => job["runs-on"] !== "ubuntu-latest").map(({ at }) => at)).toEqual([]);
		expect(jobs.filter(({ job }) => job.strategy !== undefined).map(({ at }) => at)).toEqual([]);
	});

	test("do not chain on each other with workflow_run", () => {
		expect(all.filter(workflow => "workflow_run" in workflow.on).map(workflow => workflow.file)).toEqual([]);
	});

	test("run nothing on a branch push: only release.yml has a push trigger, and only for tags", () => {
		const withPush = all.filter(workflow => "push" in workflow.on).map(workflow => workflow.file);
		expect(withPush).toEqual(["release.yml"]);
	});

	test("manual-*.yml workflows run only by hand", () => {
		const manual = all.filter(workflow => workflow.file.startsWith("manual-"));
		expect(manual.filter(workflow => Object.keys(workflow.on).join() !== "workflow_dispatch").map(workflow => workflow.file)).toEqual([]);
	});

	test("give every job a name that is unique across the workflows, so required checks are unambiguous", () => {
		const names = all.flatMap(workflow => Object.entries(workflow.jobs).map(([id, job]) => String(job.name ?? id)));
		expect(new Set(names).size).toBe(names.length);
	});

	test("do not use the cached result of an earlier run as a release gate", () => {
		expect(all.filter(workflow => /release_ci_gate|actions\/workflows\/.*\/runs/.test(workflow.text)).map(workflow => workflow.file)).toEqual([]);
	});

	test("run the Go tests before golangci-lint, build the linter with go install when its binary download failed, and lint once", () => {
		const lintJobs = all.flatMap(workflow =>
			Object.entries(workflow.jobs)
				.map(([name, job]) => ({ at: `${workflow.file}: ${name}`, steps: stepsOf(job) }))
				.filter(({ steps }) => steps.some(step => String(step.uses ?? "").startsWith("golangci/"))),
		);
		expect(lintJobs.length).toBeGreaterThan(0);
		for (const { at, steps } of lintJobs) {
			const lints = steps.filter(step => String(step.uses ?? "").startsWith("golangci/"));
			const testStep = steps.findIndex(step => String(step.run ?? "").startsWith("go test"));
			const [binary, fallback, lint] = lints.map((step): Json & { with: Json } => ({ ...step, with: asRecord(step.with) }));
			const fallbackRunsOnFailure = new RegExp(`steps\\.${String(binary?.id)}\\.outcome\\s*==\\s*'failure'`).test(String(fallback?.if));
			expect({
				at,
				testsFirst: testStep >= 0 && testStep < steps.indexOf(lints[0] ?? {}),
				binaryMayFail: binary?.["continue-on-error"] === true && binary.with["install-only"] === true,
				fallback: fallbackRunsOnFailure && fallback?.with["install-mode"] === "goinstall" && fallback.with["install-only"] === true,
				sameVersion: fallback?.with.version === binary?.with.version,
				lintsOnceWithTheInstalledBinary: lints.length === 3 && lint?.with["install-mode"] === "none",
			}).toEqual({ at, testsFirst: true, binaryMayFail: true, fallback: true, sameVersion: true, lintsOnceWithTheInstalledBinary: true });
		}
	});
});

describe.each(reduced.map(workflow => [workflow.file, workflow] as const))("reduced suite %s", (_file, workflow) => {
	test("runs for pull requests into develop, release/** and hotfix/** (and by hand), never for pushes", () => {
		const pullRequest = asRecord(workflow.on.pull_request);
		expect(pullRequest.branches).toEqual(REDUCED_BASES);
		expect(pullRequest.types).toContain("ready_for_review");
		expect(Object.keys(workflow.on).sort()).toEqual(["pull_request", "workflow_dispatch"]);
	});

	test("is path-filtered and counts its own workflow file", () => {
		const paths = asRecord(workflow.on.pull_request).paths;
		expect(Array.isArray(paths) && paths.length > 0).toBe(true);
		const own = `.github/workflows/${workflow.file}`;
		expect((paths as string[]).includes(own) || (paths as string[]).includes(".github/workflows/**")).toBe(true);
	});

	test("skips draft pull requests and cancels superseded runs of the same pull request", () => {
		expect(workflow.concurrency?.["cancel-in-progress"]).toBe(true);
		expect(workflow.concurrency?.group).toBe(CONCURRENCY_GROUP);
		for (const job of Object.values(workflow.jobs)) expect(job.if).toBe(DRAFT_GUARD);
	});

	test("does not use the race detector", () => {
		expect(runsOf(workflow).some(run => run.includes("-race"))).toBe(false);
	});
});

describe("full suite full.yml", () => {
	const full = loadWorkflow("full.yml");

	test("runs for pull requests into main (and by hand), never for any other base or a push", () => {
		const pullRequest = asRecord(full.on.pull_request);
		expect(pullRequest.branches).toEqual(["main"]);
		expect(pullRequest.types).toContain("ready_for_review");
		expect(Object.keys(full.on).sort()).toEqual(["pull_request", "workflow_dispatch"]);
		expect(JSON.stringify(full.on)).not.toMatch(/develop|feature|hotfix|release\//);
	});

	test("is never path-filtered, so its jobs always report and can be required checks of main", () => {
		const pullRequest = asRecord(full.on.pull_request);
		expect(pullRequest.paths).toBeUndefined();
		expect(pullRequest["paths-ignore"]).toBeUndefined();
	});

	test("skips draft pull requests and cancels superseded runs of the same pull request", () => {
		expect(full.concurrency?.["cancel-in-progress"]).toBe(true);
		expect(full.concurrency?.group).toBe(CONCURRENCY_GROUP);
		for (const job of Object.values(full.jobs)) expect(job.if).toBe(DRAFT_GUARD);
	});

	test("covers every area, the race detector, the real bundle and the packaging build, under full-* job names", () => {
		expect(Object.keys(full.jobs).sort()).toEqual(["go", "package", "python", "typescript", "web"]);
		for (const [id, job] of Object.entries(full.jobs)) expect(job.name).toBe(`full-${id}`);
		expect(runsOf(full).some(run => run.includes("go test -race"))).toBe(true);
		expect(runsOf(full).some(run => run.includes("go test ./internal/web/"))).toBe(true);
		expect(runsOf(full).some(run => run.includes("scripts/build.py"))).toBe(true);
	});
});

describe("publisher release.yml", () => {
	const release = loadWorkflow("release.yml");

	test("runs only for v* tags", () => {
		expect(release.on).toEqual({ push: { tags: ["v*"] } });
	});

	test("checks the tag first, then builds and publishes, with no suite and no CI gate", () => {
		const runs = runsOf(release);
		expect(runs[0]).toContain("release_version.py");
		expect(runs.some(run => run.includes("scripts/build.py"))).toBe(true);
		expect(runs.some(run => run.includes("gh release create"))).toBe(true);
		expect(runs.some(run => /go test|bun test|pytest|gh api/.test(run))).toBe(false);
	});

	test("marks a pre-release tag as a GitHub pre-release, so it never becomes the latest release", () => {
		expect(release.text).toContain("--prerelease");
	});
});
