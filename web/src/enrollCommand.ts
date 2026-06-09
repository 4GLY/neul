export type PortableShellInvocation = {
	readonly args: readonly ["-c", string];
	readonly file: "/bin/sh";
};

export function buildPortableShellInvocation(
	command: string,
): PortableShellInvocation {
	return {
		args: ["-c", command],
		file: "/bin/sh",
	};
}

export function buildEnrollCommand(
	generated: string,
	configDir: string,
): string {
	return generated.replace(
		" --connect-once",
		` --config-dir ${shellQuote(configDir)} --connect-once`,
	);
}

function shellQuote(value: string): string {
	return `'${value.replaceAll("'", "'\\''")}'`;
}
