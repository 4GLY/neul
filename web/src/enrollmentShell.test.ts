import { describe, expect, it } from "vitest";
import { buildEnrollmentShellInvocation } from "./enrollmentShell";

describe("buildEnrollmentShellInvocation", () => {
	it("runs generated enrollment commands through the portable POSIX shell", () => {
		const invocation = buildEnrollmentShellInvocation(
			"go run ./cmd/neul agent enroll",
		);

		expect(invocation).toEqual({
			args: ["-c", "go run ./cmd/neul agent enroll"],
			file: "/bin/sh",
		});
	});
});
