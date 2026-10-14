import type { Platform, RunStatus, Transport } from "../shared/types.ts";
import windowsScript from "./collect.ps1" with { type: "text" };
import posixScript from "./collect.sh" with { type: "text" };

export const SSH_OPTIONS = [
	"-T",
	"-o",
	"BatchMode=yes",
	"-o",
	"ConnectTimeout=10",
	"-o",
	"StrictHostKeyChecking=accept-new", // TOFU on the tailnet; a changed key still fails
	"-o",
	"ServerAliveInterval=10",
	"-o",
	"ServerAliveCountMax=2",
];

export const COLLECT_TIMEOUT_MS = 60_000;

export interface Target {
	alias: string;
	transport: Transport;
	platform: Platform;
}

export interface ProcessResult {
	exitCode: number | null; // null = killed by timeout
	stdout: string;
	stderr: string;
}

async function spawnWithInput(
	cmd: string[],
	input: string,
	timeoutMs: number,
): Promise<ProcessResult> {
	const proc = Bun.spawn(cmd, {
		stdin: new Blob([input]),
		stdout: "pipe",
		stderr: "pipe",
		cwd: process.env.HOME,
		timeout: timeoutMs,
		killSignal: "SIGKILL",
	});
	const [stdout, stderr, exitCode] = await Promise.all([
		new Response(proc.stdout).text(),
		new Response(proc.stderr).text(),
		proc.exited,
	]);
	return { exitCode: proc.signalCode ? null : exitCode, stdout, stderr };
}

/** Collector arguments: the repo count, the repos, then the tracked path patterns. */
function collectCommand(
	target: Target,
	trackedPaths: string[],
	repos: string[],
): string[] {
	if (target.platform === "windows") {
		return [
			"ssh",
			...SSH_OPTIONS,
			target.alias,
			"powershell -NoProfile -NonInteractive -Command -",
		];
	}
	const args = [String(repos.length), ...repos, ...trackedPaths];
	if (target.transport === "local") return ["sh", "-s", "--", ...args];
	// The remote side joins arguments into one shell command line, so quote each argument for sh
	// while leaving globs unexpanded until the script loops over them.
	const quoted = args.map((p) => `'${p.replaceAll("'", `'\\''`)}'`);
	return ["ssh", ...SSH_OPTIONS, target.alias, "sh", "-s", "--", ...quoted];
}

export async function runCollector(
	target: Target,
	trackedPaths: string[],
	repos: string[],
	timeoutMs = COLLECT_TIMEOUT_MS,
): Promise<ProcessResult> {
	const script = target.platform === "windows" ? windowsScript : posixScript;
	return spawnWithInput(
		collectCommand(target, trackedPaths, repos),
		script,
		timeoutMs,
	);
}

/** Map a finished collector process to a run status before parsing (design.md §4.2). */
export function processStatus(
	target: Target,
	result: ProcessResult,
): { status: RunStatus; error: string | null } | null {
	if (result.exitCode === null) {
		return {
			status: "error",
			error: `수집 시간 초과 (${COLLECT_TIMEOUT_MS / 1000}s)`,
		};
	}
	if (target.transport === "ssh" && result.exitCode === 255) {
		return {
			status: "unreachable",
			error: lastLine(result.stderr) ?? "ssh 접속 실패",
		};
	}
	if (result.exitCode !== 0) {
		return {
			status: "error",
			error: lastLine(result.stderr) ?? `종료 코드 ${result.exitCode}`,
		};
	}
	return null;
}

function lastLine(text: string): string | null {
	const lines = text.trim().split("\n").filter(Boolean);
	return lines.at(-1)?.trim() ?? null;
}

/**
 * Check that an ssh alias is reachable non-interactively and detect its platform.
 * POSIX shells print the marker and `uname -s`; cmd.exe echoes the whole line and
 * PowerShell fails on `uname`, both of which mean Windows.
 */
export async function probe(alias: string): Promise<Platform> {
	const result = await spawnWithInput(
		["ssh", ...SSH_OPTIONS, alias, "echo neul-probe; uname -s"],
		"",
		20_000,
	);
	if (result.exitCode === null)
		throw new Error(`${alias}: 접속 확인 시간 초과`);
	if (result.exitCode === 255) {
		throw new Error(`${alias}: ${lastLine(result.stderr) ?? "ssh 접속 실패"}`);
	}
	const lines = result.stdout.trim().split(/\r?\n/);
	if (
		result.exitCode === 0 &&
		lines[0] === "neul-probe" &&
		/^(Darwin|Linux|\w*BSD)$/.test(lines[1] ?? "")
	) {
		return "posix";
	}
	if (result.stdout.includes("neul-probe")) return "windows";
	throw new Error(
		`${alias}: 플랫폼을 판별할 수 없습니다 (${lastLine(result.stderr) ?? "출력 없음"})`,
	);
}
