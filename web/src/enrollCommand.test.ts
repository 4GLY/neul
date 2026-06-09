import { describe, expect, it } from "vitest";
import { buildEnrollCommand, EnrollCommandError } from "./enrollCommand";

describe("enroll command execution", () => {
	it("injects the config dir before connect-once using shell-safe quoting", () => {
		const command = buildEnrollCommand(
			"go run ./cmd/neul agent enroll --pair abc --connect-once",
			"/tmp/neul config/owner's laptop",
		);

		expect(command).toBe(
			"go run ./cmd/neul agent enroll --pair abc --config-dir '/tmp/neul config/owner'\\''s laptop' --connect-once",
		);
	});

	it("fails before running a command that cannot receive the temp config dir", () => {
		expect(() =>
			buildEnrollCommand(
				"go run ./cmd/neul agent enroll --pair abc",
				"/tmp/neul config",
			),
		).toThrow(EnrollCommandError);
	});
});
