import { describe, expect, it } from "vitest";
import {
	buildEnrollCommand,
	buildPortableShellInvocation,
} from "./enrollCommand";

describe("enroll command execution", () => {
	it("runs generated commands through the portable POSIX shell", () => {
		const invocation = buildPortableShellInvocation(
			"go run ./cmd/neul agent enroll --connect-once",
		);

		expect(invocation).toEqual({
			args: ["-c", "go run ./cmd/neul agent enroll --connect-once"],
			file: "/bin/sh",
		});
		expect(invocation.file).not.toBe("zsh");
	});

	it("injects the config dir before connect-once using shell-safe quoting", () => {
		const command = buildEnrollCommand(
			"go run ./cmd/neul agent enroll --pair abc --connect-once",
			"/tmp/neul config/owner's laptop",
		);

		expect(command).toBe(
			"go run ./cmd/neul agent enroll --pair abc --config-dir '/tmp/neul config/owner'\\''s laptop' --connect-once",
		);
	});
});
