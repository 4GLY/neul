import type { FormEvent } from "react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type {
	ExecEvent,
	ExecRunDetail,
	ExecRunSummary,
	ExecStatus,
	ExecTargetDetail,
	MachineSummary,
	SavedCommand,
} from "../shared/types";
import {
	api,
	Empty,
	ErrorNotice,
	errorMessage,
	Loading,
	PageHeading,
	Spinner,
	Status,
	useResource,
} from "./ui";

const DEFAULT_TIMEOUT_S = 300;

interface Draft {
	command: string;
	machineIds: number[];
	timeoutS: number;
}

// "다시 실행" hands the previous run to the exec form across a hash navigation.
let pendingDraft: Draft | null = null;

const execStatus: Record<ExecStatus, { label: string; tone: string }> = {
	running: { label: "실행 중", tone: "collecting" },
	ok: { label: "성공", tone: "online" },
	failed: { label: "실패", tone: "error" },
	unreachable: { label: "연결 불가", tone: "unreachable" },
	timeout: { label: "시간 초과", tone: "warning" },
	canceled: { label: "취소됨", tone: "unknown" },
	interrupted: { label: "중단됨", tone: "warning" },
};

function ExecStatusLabel({ status }: { status: ExecStatus }) {
	const { label, tone } = execStatus[status];
	return (
		<span className={`status ${tone}`}>
			{status === "running" ? <Spinner /> : <span className="status-dot" />}
			{label}
		</span>
	);
}

function formatTime(value: string) {
	return new Date(value).toLocaleString("ko-KR");
}

function duration(startedAt: string, finishedAt: string | null, now: number) {
	const end = finishedAt ? Date.parse(finishedAt) : now;
	return `${(Math.max(0, end - Date.parse(startedAt)) / 1000).toFixed(1)}s`;
}

export function ExecPage() {
	const machines = useResource<MachineSummary[]>("/api/machines");
	const saved = useResource<SavedCommand[]>("/api/commands");
	const runs = useResource<ExecRunSummary[]>("/api/exec", 5000);
	const [draft] = useState(() => {
		const initial = pendingDraft;
		pendingDraft = null;
		return initial;
	});
	const [selected, setSelected] = useState<number[]>(draft?.machineIds ?? []);
	const [command, setCommand] = useState(draft?.command ?? "");
	const [timeoutS, setTimeoutS] = useState(
		String(draft?.timeoutS ?? DEFAULT_TIMEOUT_S),
	);
	const [name, setName] = useState("");
	const [confirming, setConfirming] = useState(false);
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const [savedMessage, setSavedMessage] = useState("");
	const confirmRef = useRef<HTMLHeadingElement>(null);

	const available = (machines.data ?? []).filter(
		(m) => m.enabled || selected.includes(m.id),
	);
	const targets = available.filter((m) => selected.includes(m.id));
	const existing = saved.data?.find((c) => c.name === name.trim());

	useEffect(() => {
		if (confirming) confirmRef.current?.focus();
	}, [confirming]);

	function toggle(id: number) {
		setSelected((current) =>
			current.includes(id) ? current.filter((x) => x !== id) : [...current, id],
		);
	}

	function review(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setError("");
		setSavedMessage("");
		if (targets.length === 0)
			return setError("실행할 머신을 하나 이상 고르세요.");
		if (!command.trim()) return setError("명령을 입력하세요.");
		const seconds = Number(timeoutS);
		if (!Number.isInteger(seconds) || seconds < 1 || seconds > 3600)
			return setError("제한 시간은 1~3600초 사이 정수여야 합니다.");
		setConfirming(true);
	}

	async function run() {
		setBusy(true);
		setError("");
		try {
			const { id } = await api<{ id: number }>("/api/exec", "POST", {
				command,
				machineIds: targets.map((m) => m.id),
				timeoutS: Number(timeoutS),
			});
			window.location.hash = `#/exec/${id}`;
		} catch (cause) {
			setError(errorMessage(cause));
			setConfirming(false);
		} finally {
			setBusy(false);
		}
	}

	async function save() {
		setError("");
		setSavedMessage("");
		if (!name.trim()) return setError("저장할 이름을 입력하세요.");
		if (!command.trim()) return setError("저장할 명령을 입력하세요.");
		setBusy(true);
		try {
			const body = { name: name.trim(), command };
			await (existing
				? api(`/api/commands/${existing.id}`, "PATCH", body)
				: api("/api/commands", "POST", body));
			setSavedMessage(existing ? "덮어썼습니다." : "저장했습니다.");
			await saved.refresh();
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}

	async function remove(item: SavedCommand) {
		if (!window.confirm(`저장된 명령 “${item.name}”을(를) 삭제할까요?`)) return;
		try {
			await api(`/api/commands/${item.id}`, "DELETE");
			await saved.refresh();
		} catch (cause) {
			setError(errorMessage(cause));
		}
	}

	function load(item: SavedCommand) {
		setCommand(item.command);
		setName(item.name);
		setConfirming(false);
		setSavedMessage("");
		document.getElementById("exec-command")?.focus();
	}

	return (
		<>
			<PageHeading
				eyebrow="REMOTE EXEC"
				title="원격 실행"
				description="고른 머신에서 셸 명령을 실행하고 머신별 출력을 실시간으로 봅니다."
			/>
			<section className="panel" aria-labelledby="exec-form-title">
				<div className="section-heading">
					<h2 id="exec-form-title">새 실행</h2>
					<span className="muted small">
						실행 전에 대상과 명령을 한 번 더 확인합니다.
					</span>
				</div>
				<form onSubmit={review}>
					<fieldset disabled={busy || confirming} className="exec-form">
						<div className="exec-targets">
							<div className="exec-targets-heading">
								<span className="label-text">
									대상 머신 <span className="required">필수</span>
								</span>
								<button
									type="button"
									className="quiet small-button"
									onClick={() =>
										setSelected(
											targets.length === available.length
												? []
												: available.map((m) => m.id),
										)
									}
									disabled={available.length === 0}
								>
									{targets.length === available.length && available.length
										? "모두 해제"
										: "모두 선택"}
								</button>
							</div>
							{machines.loading ? (
								<Loading />
							) : available.length ? (
								available.map((m) => (
									<label key={m.id} className="exec-target">
										<input
											type="checkbox"
											checked={selected.includes(m.id)}
											onChange={() => toggle(m.id)}
										/>
										<span className="exec-target-name">{m.name}</span>
										<span className="muted small mono">
											{m.transport === "local" ? "local" : m.platform}
										</span>
										<span className="exec-target-status">
											<Status status={m.status} />
										</span>
									</label>
								))
							) : (
								<p className="muted small">
									활성 머신이 없습니다. <a href="#/">머신 추가</a>
								</p>
							)}
							<ErrorNotice
								message={machines.error}
								retry={() => void machines.refresh()}
							/>
						</div>
						<div className="exec-fields">
							<label htmlFor="exec-command">
								<span>
									명령 <span className="required">필수</span>
								</span>
								<textarea
									id="exec-command"
									className="command-input"
									rows={6}
									spellCheck={false}
									autoComplete="off"
									value={command}
									onChange={(e) => setCommand(e.target.value)}
									placeholder={"brew outdated\nmise outdated"}
								/>
							</label>
							<div className="exec-fields-row">
								<label>
									제한 시간 (초)
									<input
										type="number"
										inputMode="numeric"
										min={1}
										max={3600}
										value={timeoutS}
										onChange={(e) => setTimeoutS(e.target.value)}
										className="mono"
									/>
								</label>
								<label>
									저장 이름
									<input
										autoComplete="off"
										value={name}
										onChange={(e) => setName(e.target.value)}
										placeholder="자주 쓰는 명령으로 저장"
									/>
								</label>
							</div>
						</div>
					</fieldset>
					<ErrorNotice message={error} />
					{confirming ? (
						<section
							className="exec-confirm"
							aria-labelledby="exec-confirm-title"
						>
							<h3 id="exec-confirm-title" tabIndex={-1} ref={confirmRef}>
								{targets.length}대에서 아래 명령을 실행합니다
							</h3>
							<ul className="target-chips">
								{targets.map((m) => (
									<li key={m.id} className="mono">
										{m.name}
									</li>
								))}
							</ul>
							<pre className="exec-command">{command}</pre>
							<div className="form-footer exec-confirm-footer">
								<p className="muted small">
									머신마다 제한 시간 {timeoutS}초. 실행하면 바로 시작됩니다.
								</p>
								<div className="actions">
									<button
										type="button"
										onClick={() => setConfirming(false)}
										disabled={busy}
									>
										수정
									</button>
									<button
										type="button"
										className="primary"
										onClick={() => void run()}
										disabled={busy}
									>
										{busy ? <Spinner /> : null}
										{targets.length}대에서 실행
									</button>
								</div>
							</div>
						</section>
					) : (
						<div className="form-footer">
							<p className="muted small">
								비대화형 <span className="mono">sh</span>에서
								실행됩니다(Windows는 PowerShell). sudo 비밀번호 입력이나 대화형
								명령은 쓸 수 없습니다.
							</p>
							<div className="actions">
								{savedMessage ? (
									<span className="success-text small" role="status">
										{savedMessage}
									</span>
								) : null}
								<button
									type="button"
									onClick={() => void save()}
									disabled={busy}
								>
									{existing ? "덮어쓰기" : "명령 저장"}
								</button>
								<button type="submit" className="primary" disabled={busy}>
									실행 확인
								</button>
							</div>
						</div>
					)}
				</form>
			</section>

			<section className="panel" aria-labelledby="saved-title">
				<div className="section-heading">
					<div className="row-title">
						<h2 id="saved-title">저장된 명령</h2>
						<span className="count-label mono">{saved.data?.length ?? 0}</span>
					</div>
					<span className="muted small">불러온 뒤 확인을 거쳐 실행합니다.</span>
				</div>
				<ErrorNotice message={saved.error} retry={() => void saved.refresh()} />
				{saved.loading ? (
					<Loading />
				) : saved.data?.length ? (
					<div className="table-scroll runs-scroll">
						<table>
							<thead>
								<tr>
									<th scope="col">이름</th>
									<th scope="col">명령</th>
									<th scope="col">
										<span className="sr-only">동작</span>
									</th>
								</tr>
							</thead>
							<tbody>
								{saved.data.map((item) => (
									<tr key={item.id}>
										<th scope="row" className="row-key nowrap">
											{item.name}
										</th>
										<td>
											<code className="one-line mono">{item.command}</code>
										</td>
										<td>
											<div className="actions row-actions">
												<button
													type="button"
													className="small-button"
													onClick={() => load(item)}
												>
													불러오기
												</button>
												<button
													type="button"
													className="small-button danger"
													onClick={() => void remove(item)}
													aria-label={`${item.name} 삭제`}
												>
													삭제
												</button>
											</div>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				) : (
					<Empty title="저장된 명령이 없습니다.">
						<p>명령과 저장 이름을 입력하고 “명령 저장”을 누르세요.</p>
					</Empty>
				)}
			</section>

			<section className="panel" aria-labelledby="history-title">
				<div className="section-heading">
					<h2 id="history-title">실행 기록</h2>
					<span className="muted small">최근 {runs.data?.length ?? 0}건</span>
				</div>
				<ErrorNotice message={runs.error} retry={() => void runs.refresh()} />
				{runs.loading ? (
					<Loading />
				) : runs.data?.length ? (
					<div className="table-scroll inventory-scroll">
						<table>
							<thead>
								<tr>
									<th scope="col">시작</th>
									<th scope="col">명령</th>
									<th scope="col">대상</th>
									<th scope="col">실행자</th>
								</tr>
							</thead>
							<tbody>
								{runs.data.map((r) => (
									<tr key={r.id}>
										<td className="nowrap">
											<a href={`#/exec/${r.id}`}>
												<time dateTime={r.createdAt}>
													{formatTime(r.createdAt)}
												</time>
											</a>
										</td>
										<td>
											<code className="one-line mono">{r.command}</code>
										</td>
										<td>
											<ul className="target-chips">
												{r.targets.map((t) => (
													<li key={t.machineName}>
														<span className="mono">{t.machineName}</span>{" "}
														<ExecStatusLabel status={t.status} />
													</li>
												))}
											</ul>
										</td>
										<td className="muted mono">{r.createdBy}</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				) : (
					<Empty title="실행 기록이 없습니다." />
				)}
			</section>
		</>
	);
}

type Received<T extends ExecEvent["type"]> = Extract<ExecEvent, { type: T }>;

function withTarget(
	run: ExecRunDetail,
	machineId: number,
	update: (target: ExecTargetDetail) => ExecTargetDetail,
): ExecRunDetail {
	return {
		...run,
		targets: run.targets.map((t) =>
			t.machineId === machineId ? update(t) : t,
		),
	};
}

/** Follow a run over SSE; every (re)connection starts from a full snapshot. */
function useExecStream(id: number) {
	const [run, setRun] = useState<ExecRunDetail | null>(null);
	const [error, setError] = useState("");
	const [reconnecting, setReconnecting] = useState(false);
	useEffect(() => {
		setRun(null);
		setError("");
		const source = new EventSource(`/api/exec/${id}/events`);
		const on = <T extends ExecEvent["type"]>(
			type: T,
			handle: (event: Received<T>) => void,
		) =>
			source.addEventListener(type, (message) =>
				handle(JSON.parse((message as MessageEvent<string>).data)),
			);
		on("snapshot", (event) => {
			setRun(event.run);
			setReconnecting(false);
		});
		on("chunk", ({ machineId, stream, data }) =>
			setRun((current) =>
				current
					? withTarget(current, machineId, (t) => {
							const last = t.output.at(-1);
							return {
								...t,
								output:
									last?.stream === stream
										? [
												...t.output.slice(0, -1),
												{ stream, data: last.data + data },
											]
										: [...t.output, { stream, data }],
							};
						})
					: current,
			),
		);
		on("exit", ({ machineId, target }) =>
			setRun((current) =>
				current
					? withTarget(current, machineId, (t) => ({
							...target,
							output: t.output,
						}))
					: current,
			),
		);
		on("done", ({ finishedAt }) => {
			setRun((current) => (current ? { ...current, finishedAt } : current));
			source.close();
		});
		source.onerror = () => {
			if (source.readyState !== EventSource.CLOSED) {
				setReconnecting(true);
				return;
			}
			// The server answered with an error instead of a stream; fetch it for the message.
			api(`/api/exec/${id}`).then(
				() => setError("실시간 연결을 열지 못했습니다. 새로고침하세요."),
				(cause) => setError(errorMessage(cause)),
			);
		};
		return () => source.close();
	}, [id]);
	return { run, error, reconnecting };
}

function useNow(active: boolean) {
	const [now, setNow] = useState(Date.now());
	useEffect(() => {
		if (!active) return;
		const timer = window.setInterval(() => setNow(Date.now()), 1000);
		return () => window.clearInterval(timer);
	}, [active]);
	return now;
}

export function ExecRunView({ id }: { id: number }) {
	const { run, error, reconnecting } = useExecStream(id);
	const running = run ? run.finishedAt === null : false;
	const now = useNow(running);
	const [canceling, setCanceling] = useState(false);
	const [actionError, setActionError] = useState("");

	async function cancel() {
		setCanceling(true);
		setActionError("");
		try {
			await api(`/api/exec/${id}/cancel`, "POST");
		} catch (cause) {
			setActionError(errorMessage(cause));
		} finally {
			setCanceling(false);
		}
	}

	function rerun() {
		if (!run) return;
		pendingDraft = {
			command: run.command,
			machineIds: run.targets.flatMap((t) =>
				t.machineId === null ? [] : [t.machineId],
			),
			timeoutS: run.timeoutS,
		};
		window.location.hash = "#/exec";
	}

	return (
		<>
			<a className="back-link" href="#/exec">
				← 원격 실행
			</a>
			<ErrorNotice message={error} />
			{!run ? (
				error ? null : (
					<Loading />
				)
			) : (
				<>
					<PageHeading
						eyebrow={`EXEC #${run.id}`}
						title={running ? "실행 중" : "실행 결과"}
						description={`${formatTime(run.createdAt)} · ${run.createdBy} · 머신별 제한 시간 ${run.timeoutS}초`}
					>
						{running ? (
							<button
								type="button"
								className="danger"
								onClick={() => void cancel()}
								disabled={canceling}
							>
								{canceling ? <Spinner /> : null}
								실행 취소
							</button>
						) : null}
						<button type="button" onClick={rerun}>
							다시 실행
						</button>
					</PageHeading>
					<ErrorNotice message={actionError} />
					{reconnecting && running ? (
						<div className="notice" role="status">
							<span>
								<Spinner /> 연결이 끊겨 다시 연결하는 중…
							</span>
						</div>
					) : null}
					<section className="panel" aria-labelledby="command-title">
						<div className="section-heading">
							<h2 id="command-title">명령</h2>
							<span className="muted small">
								{run.targets.length}대
								{run.finishedAt
									? ` · ${duration(run.createdAt, run.finishedAt, now)}`
									: ""}
							</span>
						</div>
						<pre className="exec-command exec-command-block">{run.command}</pre>
					</section>
					<div className="exec-outputs">
						{run.targets.map((t) => (
							<OutputPanel
								key={t.machineId ?? t.machineName}
								target={t}
								now={now}
							/>
						))}
					</div>
				</>
			)}
		</>
	);
}

function OutputPanel({
	target,
	now,
}: {
	target: ExecTargetDetail;
	now: number;
}) {
	const ref = useRef<HTMLPreElement>(null);
	const stick = useRef(true);
	// Follow new output while the reader is at the bottom; leave the scroll alone otherwise.
	useLayoutEffect(() => {
		const el = ref.current;
		if (el && stick.current) el.scrollTop = el.scrollHeight;
	});
	const titleId = `output-${target.machineName}`;
	return (
		<section className="panel" aria-labelledby={titleId}>
			<div className="section-heading">
				<div className="row-title">
					<h2 id={titleId}>{target.machineName}</h2>
					<ExecStatusLabel status={target.status} />
				</div>
				<span className="muted small mono">
					{target.exitCode === null ? "" : `exit ${target.exitCode} · `}
					{duration(target.startedAt, target.finishedAt, now)}
				</span>
			</div>
			<pre
				ref={ref}
				className="exec-output"
				onScroll={(e) => {
					const el = e.currentTarget;
					stick.current = el.scrollTop + el.clientHeight >= el.scrollHeight - 8;
				}}
			>
				{target.output.length ? (
					target.output.map((chunk, i) => (
						<span
							// Chunks only ever append, so the index is a stable key.
							// biome-ignore lint/suspicious/noArrayIndexKey: append-only list
							key={i}
							className={chunk.stream}
						>
							{chunk.data.replace(/\r\n?/g, "\n")}
						</span>
					))
				) : (
					<span className="muted">
						{target.status === "running" ? "출력을 기다리는 중…" : "출력 없음"}
					</span>
				)}
			</pre>
			{target.truncated ? (
				<p className="table-meta muted small">
					출력이 1 MiB를 넘어 이후 내용은 저장하지 않았습니다.
				</p>
			) : null}
		</section>
	);
}
