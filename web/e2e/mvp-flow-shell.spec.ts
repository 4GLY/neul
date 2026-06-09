import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { enrollmentShellPath, withCommonGoPath } from "./mvp-flow";
import { repoRoot } from "./server-fixture";

test("MVP flow enrollment command uses portable POSIX shell", () => {
	// Given
	const source = readFileSync(join(repoRoot, "web/e2e/mvp-flow.ts"), "utf8");

	// Then
	expect(enrollmentShellPath).toBe("/bin/sh");
	expect(source).toMatch(/execFileSync\s*\(\s*enrollmentShellPath/);
	expect(source).toMatch(
		/runAgentTick[\s\S]*env:\s*processEnvForEnrollment\(\)/,
	);
});

test("MVP flow enrollment environment keeps common Go paths", () => {
	// Given
	const path = withCommonGoPath("/workspace/bin:/usr/local/bin:");

	// Then
	expect(path.split(":")).toEqual([
		"/workspace/bin",
		"/usr/local/bin",
		"/opt/homebrew/bin",
		"/usr/local/go/bin",
	]);
});
