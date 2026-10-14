import { describe, expect, test } from "bun:test";
import { mkdtempSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Secrets } from "../src/server/secrets.ts";

describe("Secrets", () => {
	test("round-trips through a new 600 identity without leaking the plaintext", async () => {
		const path = join(
			mkdtempSync(join(tmpdir(), "neul-age-")),
			"sub",
			"id.txt",
		);
		const plain = "-----BEGIN KEY-----\nnot-a-real-key ✓\n";
		const armored = await new Secrets(path).encrypt(plain);

		expect(statSync(path).mode & 0o777).toBe(0o600);
		expect(armored.startsWith("-----BEGIN AGE ENCRYPTED FILE-----")).toBe(true);
		expect(armored).not.toContain("not-a-real-key");
		// A fresh instance reads the same identity file back.
		expect(await new Secrets(path).decrypt(armored)).toBe(plain);
	});
});
