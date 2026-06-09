import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { withCommonGoPath } from "./mvp-flow";
import { repoRoot } from "./server-fixture";

test("MVP flow enrollment command uses portable POSIX shell", () => {
	// Given
	const source = readFileSync(join(repoRoot, "web/e2e/mvp-flow.ts"), "utf8");

	// Then
	expect(source).not.toMatch(/execFileSync\s*\(\s*["']zsh["']/);
	expect(source).toMatch(/execFileSync\s*\(\s*["']\/bin\/sh["']/);
});

test("MVP flow enrollment environment keeps common Go paths", () => {
	// Given
	const path = withCommonGoPath("/workspace/bin");

	// Then
	expect(path.split(":")).toEqual(
		expect.arrayContaining([
			"/workspace/bin",
			"/opt/homebrew/bin",
			"/usr/local/bin",
			"/usr/local/go/bin",
		]),
	);
});
