import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
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
	await page.getByText("Run from your neul checkout:").waitFor();
	const generated = await page.locator("code").first().textContent();
	if (generated === null || !generated.includes("neul agent enroll")) {
		throw new Error(`generated command missing: ${generated}`);
	}
	const configDir = join(fixture.tempDir, "agent-config");
	mkdirSync(configDir, { recursive: true });
	const command = generated.replace(
		" --connect-once",
		` --config-dir ${shellQuote(configDir)} --connect-once`,
	);
	const invocation = buildEnrollmentShellInvocation(command);
	const output = execFileSync(invocation.file, invocation.args, {
		cwd: repoRoot,
		encoding: "utf8",
		env: processEnvWithoutGoroot(),
	});
	writeFileSync(
		join(evidenceDir, "task-6-agent-onboarding-e2e-log.txt"),
		[`generated=${generated}`, `executed=${command}`, output].join("\n"),
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
			env: processEnvWithoutGoroot(),
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

function shellQuote(value: string): string {
	return `'${value.replaceAll("'", "'\\''")}'`;
}
