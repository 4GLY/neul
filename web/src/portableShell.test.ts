import { describe, expect, it } from "vitest";
import { buildPortableShellInvocation } from "./portableShell";

describe("buildPortableShellInvocation", () => {
	it("uses POSIX sh so CI runners do not need zsh", () => {
		const command = "go run ./cmd/neul agent enroll --connect-once";

		expect(buildPortableShellInvocation(command)).toEqual({
			args: ["-c", command],
			executable: "/bin/sh",
		});
	});
});
