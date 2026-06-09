export type PortableShellInvocation = {
	readonly args: readonly string[];
	readonly executable: string;
};

export function buildPortableShellInvocation(
	command: string,
): PortableShellInvocation {
	return {
		args: ["-c", command],
		executable: "/bin/sh",
	};
}
