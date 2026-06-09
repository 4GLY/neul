export type PortableShellInvocation = {
	readonly args: readonly ["-c", string];
	readonly file: "/bin/sh";
};

export class EnrollCommandError extends Error {
	constructor() {
		super("generated enroll command is missing --connect-once");
		this.name = "EnrollCommandError";
	}
}

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
	const command = generated.replace(
		" --connect-once",
		` --config-dir ${shellQuote(configDir)} --connect-once`,
	);
	if (command === generated) {
		throw new EnrollCommandError();
	}
	return command;
}

function shellQuote(value: string): string {
	return `'${value.replaceAll("'", "'\\''")}'`;
}
