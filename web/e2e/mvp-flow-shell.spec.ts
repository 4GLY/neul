import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { repoRoot } from "./server-fixture";

test("MVP flow enrollment command uses portable POSIX shell", () => {
	// Given
	const source = readFileSync(join(repoRoot, "web/e2e/mvp-flow.ts"), "utf8");

	// Then
	expect(source).not.toContain('execFileSync("zsh"');
	expect(source).toContain('execFileSync("/bin/sh"');
});
