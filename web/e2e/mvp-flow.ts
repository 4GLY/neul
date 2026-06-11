import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { buildEnrollCommand } from "../src/enrollCommand";
import { buildEnrollmentShellInvocation } from "../src/enrollmentShell";
import {
	evidenceDir,
	qaDir,
	repoRoot,
	type ServerFixture,
} from "./server-fixture";

export type EnrolledMachine = {
	readonly machineId: string;
	readonly machineToken: string;
	readonly configDir: string;
	readonly configPath: string;
};

export async function visibleOnboardMachine(
	page: Page,
	fixture: ServerFixture,
): Promise<EnrolledMachine> {
	await page.goto(fixture.baseURL);
	await page.getByRole("button", { name: "첫 머신 등록" }).click();
	await page.getByText("Run with packaged neul client:").waitFor();
	await page
		.getByText(
			"packaged approval flow가 준비되기 전에는 fallback/debug 명령으로 등록하세요:",
		)
		.waitFor();
	const generated = await page.locator("code").first().textContent();
	if (
		generated === null ||
		generated.includes("--pair") ||
		generated.includes("go run") ||
		!generated.includes("neul enroll --server")
	) {
		throw new Error(`generated command missing: ${generated}`);
	}
	const fallback = await page.locator("code").nth(1).textContent();
	if (
		fallback === null ||
		!fallback.includes("go run ./cmd/neul agent enroll") ||
		!fallback.includes("--pair") ||
		!fallback.includes("--connect-once")
	) {
		throw new Error(`fallback command missing: ${fallback}`);
	}
	const configDir = join(fixture.tempDir, "agent-config");
	mkdirSync(configDir, { recursive: true });
	const command = buildEnrollCommand(fallback, configDir);
	const invocation = buildEnrollmentShellInvocation(command);
	const output = execFileSync(invocation.file, invocation.args, {
		cwd: repoRoot,
		encoding: "utf8",
		env: processEnvForEnrollment(),
	});
	writeFileSync(
		join(evidenceDir, "task-6-agent-onboarding-e2e-log.txt"),
		[
			`primary=${generated}`,
			"fallback=checkout-local enrollment for E2E fixture setup",
			`executed=${command}`,
			output,
		].join("\n"),
	);
	await page.getByText("Connected").first().waitFor({ timeout: 15_000 });
	const configPath = join(configDir, "config.json");
	const config = JSON.parse(readFileSync(configPath, "utf8")) as {
		readonly machineId: string;
		readonly machineToken: string;
	};
	return {
		machineId: config.machineId,
		machineToken: config.machineToken,
		configDir,
		configPath,
	};
}

export async function findResource(
	baseURL: string,
	session: string,
	name: string,
) {
	const response = await fetch(`${baseURL}/api/resources`, {
		headers: { Cookie: `neul_session=${session}` },
	});
	expect(response.status).toBe(200);
	const body = (await response.json()) as {
		readonly resources: readonly {
			readonly id: string;
			readonly name: string;
		}[];
	};
	const resource = body.resources.find((candidate) => candidate.name === name);
	if (resource === undefined) {
		throw new Error(`resource ${name} was not found`);
	}
	return resource;
}

export async function postDriftReport(
	baseURL: string,
	enrolled: EnrolledMachine,
	resourceId: string,
): Promise<void> {
	const response = await fetch(`${baseURL}/api/agent/drift-report`, {
		body: JSON.stringify({
			events: [
				{
					appliedVersion: 0,
					desiredVersion: 1,
					message: "brew check",
					resourceId,
					status: "drifted",
				},
			],
		}),
		headers: {
			Authorization: `Bearer ${enrolled.machineToken}`,
			"Content-Type": "application/json",
			"Idempotency-Key": "e2e-drift-1",
		},
		method: "POST",
	});
	expect(response.status).toBe(202);
}

export async function expectQueuedCommand(
	baseURL: string,
	enrolled: EnrolledMachine,
	shouldExist: boolean,
): Promise<void> {
	const response = await fetch(`${baseURL}/api/agent/commands`, {
		headers: { Authorization: `Bearer ${enrolled.machineToken}` },
	});
	expect(response.status).toBe(200);
	const body = (await response.json()) as {
		readonly commands: readonly unknown[];
	};
	if (shouldExist) {
		expect(body.commands.length).toBeGreaterThan(0);
		return;
	}
	expect(body.commands).toHaveLength(0);
}

export async function runAgentTick(enrolled: EnrolledMachine): Promise<void> {
	execFileSync(
		"go",
		["run", "./cmd/neul-agent", "--once", "--config", enrolled.configPath],
		{
			cwd: repoRoot,
			env: processEnvForEnrollment(),
			stdio: "pipe",
		},
	);
}

export function writeQaDoc(
	fixture: ServerFixture,
	enrolled: EnrolledMachine,
): void {
	writeFileSync(
		join(qaDir, "mvp-dashboard.md"),
		[
			"# MVP dashboard QA",
			"",
			"- Command: `cd web && pnpm exec playwright test e2e/mvp-dashboard.spec.ts --project=chromium`",
			`- Server: ${fixture.baseURL}`,
			`- Temp DB: ${fixture.dbPath}`,
			`- Enrolled config dir: ${enrolled.configDir}`,
			`- Seed machine: ${enrolled.machineId}`,
			"- Seed resources: brew package `kubectl`, dotfile `~/.zshrc`",
			"- Drift seed: `POST /api/agent/drift-report` with one drifted brew event",
			"- Repair: browser clicked `drift 복구`, then `go run ./cmd/neul-agent --once --config <cli-written-config>` acked the queued command",
			"- Screenshot: `evidence/task-6-agent-onboarding-e2e-browser.png`",
			"- Cleanup receipt: `evidence/task-6-agent-onboarding-cleanup.txt`",
			"",
		].join("\n"),
	);
}

function processEnvWithoutGoroot(): NodeJS.ProcessEnv {
	const next = { ...process.env };
	delete next.GOROOT;
	return next;
}

function processEnvForEnrollment(): NodeJS.ProcessEnv {
	const next = processEnvWithoutGoroot();
	next.PATH = withCommonGoPath(next.PATH);
	return next;
}

export function withCommonGoPath(path: string | undefined): string {
	return [
		...(path ?? "").split(":"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/local/go/bin",
	]
		.filter((segment, index, segments) => {
			return segment.length > 0 && segments.indexOf(segment) === index;
		})
		.join(":");
}
