import { SSH_OPTIONS } from "../collector/run.ts";
import type {
	ExecChunk,
	ExecEvent,
	ExecRunDetail,
	ExecStream,
	ExecTarget,
	ExecTargetDetail,
} from "../shared/types.ts";
import type { ExecTargetResult, MachineRow, Store } from "./db.ts";

export const EXEC_OUTPUT_LIMIT = 1024 * 1024; // bytes per target, stdout and stderr combined
export const EXEC_DEFAULT_TIMEOUT_S = 300;
export const EXEC_MAX_TIMEOUT_S = 3600;
const KILL_GRACE_MS = 5000; // after closing stdin, wait this long before SIGKILL

/**
 * Collects a target's output up to `limit` bytes. Stream decoders keep multi-byte
 * characters split across reads intact; consecutive chunks of one stream are merged.
 */
export class OutputBuffer {
	readonly chunks: ExecChunk[] = [];
	truncated = false;
	private bytes = 0;
	private readonly decoders: Record<ExecStream, TextDecoder> = {
		stdout: new TextDecoder(),
		stderr: new TextDecoder(),
	};

	constructor(private readonly limit = EXEC_OUTPUT_LIMIT) {}

	/** Append raw bytes and return the text that was kept ("" when nothing new). */
	push(stream: ExecStream, bytes: Uint8Array): string {
		if (this.truncated) return "";
		let kept = bytes;
		if (this.bytes + bytes.length > this.limit) {
			kept = bytes.subarray(0, this.limit - this.bytes);
			this.truncated = true;
		}
		this.bytes += kept.length;
		const text = this.decoders[stream].decode(kept, {
			stream: !this.truncated,
		});
		this.append(stream, text);
		return text;
	}

	/** Flush bytes held back by the decoders at end of output. */
	finish(): ExecChunk[] {
		const tail: ExecChunk[] = [];
		for (const stream of ["stdout", "stderr"] as const) {
			const data = this.decoders[stream].decode();
			if (!data) continue;
			this.append(stream, data);
			tail.push({ stream, data });
		}
		return tail;
	}

	private append(stream: ExecStream, data: string): void {
		if (!data) return;
		const last = this.chunks.at(-1);
		if (last?.stream === stream) last.data += data;
		else this.chunks.push({ stream, data });
	}
}

/**
 * POSIX wrapper sent on stdin to `sh -s`. The hub keeps stdin open while the command runs:
 * the command gets /dev/null as stdin and its own process group, and a watchdog reading the
 * hub's stdin kills that group on EOF. Closing stdin (cancel, timeout, or the hub exiting)
 * therefore stops the command and its children on the remote side too.
 */
export function posixScript(command: string): string {
	const tag = `NEUL_CMD_${crypto.randomUUID().replaceAll("-", "")}`;
	return `neul_main() {
# Same PATH additions as collect.sh: non-interactive ssh shells lack brew/mise.
PATH="$HOME/.local/share/mise/shims:$HOME/.local/bin:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
export PATH
exec 3<&0
set -m 2>/dev/null
neul_cmd=$(cat <<'${tag}'
${command}
${tag}
)
sh -c "$neul_cmd" </dev/null 3<&- &
neul_pid=$!
{ cat >/dev/null; kill -KILL -- -$neul_pid || kill -KILL $neul_pid; } <&3 >/dev/null 2>&1 &
neul_watch=$!
exec 3<&-
wait $neul_pid 2>/dev/null
neul_code=$?
kill -KILL -- -$neul_watch 2>/dev/null || kill -KILL $neul_watch 2>/dev/null
exit $neul_code
}
neul_main
`;
}

function execCommand(machine: MachineRow): string[] {
	if (machine.platform === "windows") {
		return [
			"ssh",
			...SSH_OPTIONS,
			machine.alias,
			"powershell -NoProfile -NonInteractive -Command -",
		];
	}
	if (machine.transport === "local") return ["sh", "-s"];
	return ["ssh", ...SSH_OPTIONS, machine.alias, "sh", "-s"];
}

interface LiveTarget {
	targetId: number;
	machine: MachineRow;
	target: ExecTarget;
	output: OutputBuffer;
	proc: Bun.Subprocess<"pipe", "pipe", "pipe"> | null;
	stopReason: "canceled" | "timeout" | null;
	killTimer: Timer | undefined;
}

interface LiveRun {
	run: Omit<ExecRunDetail, "targets">;
	targets: LiveTarget[];
	listeners: Set<(event: ExecEvent) => void>;
}

/** Called once a target's process has ended and its result is stored. */
export type TargetDoneListener = (
	run: { id: number; kind: "command" | "apply" },
	machine: MachineRow,
) => void;

export class Executor {
	private readonly live = new Map<number, LiveRun>();
	private readonly busy = new Map<number, number>(); // machineId -> runId

	constructor(
		private readonly store: Store,
		private readonly onTargetDone: TargetDoneListener = () => {},
	) {}

	/** Machines among `ids` that already have a running exec. */
	busyMachines(ids: number[]): number[] {
		return ids.filter((id) => this.busy.has(id));
	}

	/** Machines with a running apply (desired-state) target. */
	applyingMachines(): number[] {
		return [...this.busy].flatMap(([machineId, runId]) =>
			this.live.get(runId)?.run.kind === "apply" ? [machineId] : [],
		);
	}

	/** A target command of null runs `command`; apply runs give every machine its own script. */
	start(request: {
		kind: "command" | "apply";
		command: string;
		timeoutS: number;
		createdBy: string;
		targets: { machine: MachineRow; command: string | null }[];
	}): ExecRunDetail {
		const { run, targetIds } = this.store.createExecRun({
			...request,
			createdAt: new Date().toISOString(),
		});
		const { targets, ...base } = run;
		const live: LiveRun = {
			run: base,
			listeners: new Set(),
			targets: request.targets.map(({ machine }, i) => {
				const { output: _output, ...target } = targets[i] as ExecTargetDetail;
				return {
					targetId: targetIds[i] as number,
					machine,
					target,
					output: new OutputBuffer(),
					proc: null,
					stopReason: null,
					killTimer: undefined,
				};
			}),
		};
		this.live.set(run.id, live);
		for (const t of live.targets) {
			this.busy.set(t.machine.id, run.id);
			void this.runTarget(
				live,
				t,
				t.target.command ?? request.command,
				request.timeoutS,
			);
		}
		return run;
	}

	/** Current state of a run, including output of targets still running. */
	snapshot(id: number): ExecRunDetail | null {
		const live = this.live.get(id);
		if (!live) return this.store.getExecRun(id);
		return {
			...live.run,
			targets: live.targets.map((t) => ({
				...t.target,
				output: t.output.chunks.map((c) => ({ ...c })),
			})),
		};
	}

	/** Listen to a running exec; returns null when the run is not running in this process. */
	subscribe(
		id: number,
		listener: (event: ExecEvent) => void,
	): (() => void) | null {
		const live = this.live.get(id);
		if (!live) return null;
		live.listeners.add(listener);
		return () => live.listeners.delete(listener);
	}

	/** Stop every running target of a run. Returns false when the run is not running. */
	cancel(id: number): boolean {
		const live = this.live.get(id);
		if (!live) return false;
		for (const t of live.targets) {
			if (t.target.status === "running") this.stop(t, "canceled");
		}
		return true;
	}

	private emit(live: LiveRun, event: ExecEvent): void {
		for (const listener of live.listeners) listener(event);
	}

	/**
	 * Closing stdin lets the POSIX wrapper kill the command's process group; Windows has no
	 * wrapper, and an unresponsive ssh connection never sees the EOF, so SIGKILL follows.
	 */
	private stop(t: LiveTarget, reason: "canceled" | "timeout"): void {
		if (t.stopReason) return;
		t.stopReason = reason;
		const proc = t.proc;
		if (!proc) return;
		if (t.machine.platform === "windows") {
			proc.kill("SIGKILL");
			return;
		}
		try {
			proc.stdin.end();
		} catch {
			// already closed
		}
		t.killTimer = setTimeout(() => proc.kill("SIGKILL"), KILL_GRACE_MS);
	}

	private async pump(
		live: LiveRun,
		t: LiveTarget,
		stream: ExecStream,
		source: ReadableStream<Uint8Array>,
	): Promise<void> {
		const machineId = t.machine.id;
		for await (const bytes of source) {
			const data = t.output.push(stream, bytes);
			if (data) this.emit(live, { type: "chunk", machineId, stream, data });
		}
	}

	private async runTarget(
		live: LiveRun,
		t: LiveTarget,
		command: string,
		timeoutS: number,
	): Promise<void> {
		let exitCode: number | null = null;
		let timer: Timer | undefined;
		try {
			const windows = t.machine.platform === "windows";
			// Spawning is synchronous, so t.proc is set before start() returns and cancel() sees it.
			const proc = Bun.spawn(execCommand(t.machine), {
				stdin: "pipe",
				stdout: "pipe",
				stderr: "pipe",
				cwd: process.env.HOME,
			});
			t.proc = proc;
			timer = setTimeout(() => this.stop(t, "timeout"), timeoutS * 1000);
			// PowerShell reads `-Command -` line by line until stdin ends; UTF-8 output keeps
			// non-ASCII text intact. POSIX keeps stdin open as the cancel channel.
			proc.stdin.write(
				windows
					? `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8\r\n${command}\r\n`
					: posixScript(command),
			);
			await proc.stdin.flush();
			if (windows) await proc.stdin.end();
			const [, , code] = await Promise.all([
				this.pump(live, t, "stdout", proc.stdout),
				this.pump(live, t, "stderr", proc.stderr),
				proc.exited,
			]);
			exitCode = proc.signalCode ? null : code;
		} catch (e) {
			const message = `neul: ${e instanceof Error ? e.message : String(e)}\n`;
			const data = t.output.push("stderr", new TextEncoder().encode(message));
			if (data)
				this.emit(live, {
					type: "chunk",
					machineId: t.machine.id,
					stream: "stderr",
					data,
				});
		} finally {
			clearTimeout(timer);
			clearTimeout(t.killTimer);
		}
		for (const chunk of t.output.finish()) {
			this.emit(live, { type: "chunk", machineId: t.machine.id, ...chunk });
		}
		this.finish(live, t, exitCode);
	}

	private finish(live: LiveRun, t: LiveTarget, exitCode: number | null): void {
		const status: ExecTargetResult["status"] =
			t.stopReason ??
			(exitCode === 0
				? "ok"
				: exitCode === 255 && t.machine.transport === "ssh"
					? "unreachable"
					: "failed");
		const result: ExecTargetResult = {
			status,
			exitCode,
			finishedAt: new Date().toISOString(),
			output: t.output.chunks,
			truncated: t.output.truncated,
		};
		const runFinishedAt = this.store.finishExecTarget(
			live.run.id,
			t.targetId,
			result,
		);
		t.target = {
			...t.target,
			status,
			exitCode,
			finishedAt: result.finishedAt,
			truncated: result.truncated,
		};
		t.proc = null;
		this.busy.delete(t.machine.id);
		try {
			this.onTargetDone(live.run, t.machine);
		} catch (e) {
			console.error(`after-exec hook for ${t.machine.name} failed:`, e);
		}
		this.emit(live, {
			type: "exit",
			machineId: t.machine.id,
			target: t.target,
		});
		if (runFinishedAt) {
			live.run = { ...live.run, finishedAt: runFinishedAt };
			this.live.delete(live.run.id);
			this.emit(live, { type: "done", finishedAt: runFinishedAt });
		}
	}
}
