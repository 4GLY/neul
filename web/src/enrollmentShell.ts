export type EnrollmentShellInvocation = {
	readonly file: "/bin/sh";
	readonly args: readonly ["-c", string];
};

export function buildEnrollmentShellInvocation(
	command: string,
): EnrollmentShellInvocation {
	return {
		args: ["-c", command],
		file: "/bin/sh",
	};
}
