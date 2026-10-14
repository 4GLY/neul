import type { FormEvent } from "react";
import { useState } from "react";
import type {
	AddMachineRequest,
	MachineSummary,
	Platform,
	Transport,
} from "../shared/types";
import {
	api,
	diskPercent,
	Empty,
	ErrorNotice,
	errorMessage,
	Loading,
	Meter,
	memoryPercent,
	PageHeading,
	relativeTime,
	Spinner,
	Status,
	uptime,
	useResource,
} from "./ui";

export function Machines() {
	const {
		data: machines,
		error,
		loading,
		refresh,
	} = useResource<MachineSummary[]>("/api/machines", 15000);
	const [adding, setAdding] = useState(false);
	const [collecting, setCollecting] = useState<number[]>([]);
	const [allBusy, setAllBusy] = useState(false);
	const [actionError, setActionError] = useState("");
	async function collect(id?: number) {
		setActionError("");
		if (id === undefined) setAllBusy(true);
		else setCollecting((current) => [...current, id]);
		try {
			await api(
				id === undefined ? "/api/collect" : `/api/machines/${id}/collect`,
				"POST",
			);
		} catch (cause) {
			setActionError(errorMessage(cause));
		} finally {
			if (id === undefined) setAllBusy(false);
			else setCollecting((current) => current.filter((value) => value !== id));
			await refresh();
		}
	}
	const online =
		machines?.filter((machine) => machine.status === "online").length ?? 0;
	return (
		<>
			<PageHeading
				eyebrow="MACHINE INVENTORY"
				title="내 머신"
				description="상태와 설치된 도구를 한눈에. 15초마다 자동 새로고침합니다."
			>
				<button
					type="button"
					onClick={() => void collect()}
					disabled={allBusy || !machines?.length}
				>
					{allBusy ? <Spinner /> : null}
					{allBusy ? "전체 수집 중…" : "전체 수집"}
				</button>
				<button
					type="button"
					className="primary"
					onClick={() => setAdding(!adding)}
					aria-expanded={adding}
				>
					{adding ? "추가 닫기" : "+ 머신 추가"}
				</button>
			</PageHeading>
			<ErrorNotice message={error} retry={() => void refresh()} />
			<ErrorNotice message={actionError} />
			{adding ? (
				<AddMachine
					onAdded={() => {
						setAdding(false);
						void refresh();
					}}
					onCancel={() => setAdding(false)}
				/>
			) : null}
			{loading ? (
				<Loading />
			) : machines?.length ? (
				<>
					<div className="list-summary">
						<span>
							<strong>{machines.length}</strong>대의 머신{" "}
							<span className="separator">/</span>{" "}
							<span className="online-text">{online}대 온라인</span>
						</span>
						<span className="muted">마지막 정상 수집 상태 기준</span>
					</div>
					<div className="machine-list">
						{machines.map((machine) => {
							const busy =
								machine.collecting ||
								allBusy ||
								collecting.includes(machine.id);
							return (
								<article className="machine-row" key={machine.id}>
									<div className="machine-identity">
										<div className="row-title">
											<a
												href={`#/machines/${machine.id}`}
												className="machine-name"
											>
												{machine.name}
											</a>
											<Status status={machine.status} collecting={busy} />
										</div>
										<div className="muted mono machine-meta">
											{machine.alias} <span className="separator">·</span>{" "}
											{machine.transport}
											{!machine.enabled ? (
												<span className="disabled-label">일시 중지</span>
											) : null}
										</div>
										<div className="machine-os">
											{machine.facts?.osName ??
												machine.facts?.os ??
												"OS 정보 없음"}
											<span className="muted mono">{machine.facts?.arch}</span>
										</div>
									</div>
									<div className="machine-metrics">
										<div className="metric">
											<span className="muted">가동 시간</span>
											<strong>{uptime(machine.facts?.uptimeS)}</strong>
										</div>
										<div className="metric">
											<span className="muted">부하 · 1 / 5 / 15분</span>
											<strong className="mono">
												{machine.facts?.load
													?.map((value) => value.toFixed(2))
													.join(" / ") ?? "—"}
											</strong>
										</div>
										<div className="metric">
											<span className="muted">배터리</span>
											<strong>
												{machine.facts?.battery
													? `${machine.facts.battery.percent}%`
													: "—"}{" "}
												<span className="muted small">
													{machine.facts?.battery?.state}
												</span>
											</strong>
										</div>
									</div>
									<div className="machine-resources">
										<Meter
											label="메모리"
											value={memoryPercent(machine.facts)}
										/>
										<Meter label="디스크" value={diskPercent(machine.facts)} />
									</div>
									<div className="machine-inventory">
										{machine.counts ? (
											<div className="inventory-counts">
												{machine.counts.brewFormulae !== null ? (
													<span>
														brew <b>{machine.counts.brewFormulae}</b>
														<span className="muted small">
															{" "}
															· 직접 {machine.counts.brewLeaves ?? 0}
														</span>
													</span>
												) : null}
												{machine.counts.brewCasks !== null ? (
													<span>
														cask <b>{machine.counts.brewCasks}</b>
													</span>
												) : null}
												{machine.counts.pacmanTotal !== null ? (
													<span>
														pacman <b>{machine.counts.pacmanTotal}</b>
														<span className="muted small">
															{" "}
															· 직접 {machine.counts.pacmanExplicit ?? 0}
														</span>
													</span>
												) : null}
												{machine.counts.miseTools !== null ? (
													<span>
														mise <b>{machine.counts.miseTools}</b>
													</span>
												) : null}
												{machine.counts.files !== null ? (
													<span>
														파일 <b>{machine.counts.files}</b>
													</span>
												) : null}
											</div>
										) : (
											<span className="muted">인벤토리 없음</span>
										)}
									</div>
									<div className="machine-last">
										<time
											className="muted small"
											dateTime={machine.lastRun?.finishedAt}
											title={
												machine.lastRun
													? new Date(machine.lastRun.finishedAt).toLocaleString(
															"ko-KR",
														)
													: undefined
											}
										>
											{relativeTime(machine.lastRun?.finishedAt ?? null)}
										</time>
										<button
											type="button"
											className="small-button"
											onClick={() => void collect(machine.id)}
											disabled={busy}
											aria-label={`${machine.name} 수집`}
										>
											{busy ? <Spinner /> : null}
											{busy ? "수집 중" : "수집"}
										</button>
									</div>
									{machine.lastRun?.error ? (
										<p className="machine-error" title={machine.lastRun.error}>
											{machine.lastRun.error}
										</p>
									) : null}
								</article>
							);
						})}
					</div>
				</>
			) : !error ? (
				<Empty title="아직 등록된 머신이 없습니다.">
					<p>SSH 별칭을 추가하거나 이 허브를 local 머신으로 등록하세요.</p>
					<button
						type="button"
						className="primary"
						onClick={() => setAdding(true)}
					>
						첫 머신 추가
					</button>
				</Empty>
			) : null}
		</>
	);
}

function AddMachine({
	onAdded,
	onCancel,
}: {
	onAdded: () => void;
	onCancel: () => void;
}) {
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const values = new FormData(event.currentTarget);
		const name = String(values.get("name") ?? "").trim();
		const platform = String(values.get("platform"));
		const request: AddMachineRequest = {
			alias: String(values.get("alias") ?? "").trim(),
			transport: values.get("transport") as Transport,
			...(name ? { name } : {}),
			...(platform === "auto" ? {} : { platform: platform as Platform }),
		};
		setBusy(true);
		setError("");
		try {
			await api("/api/machines", "POST", request);
			onAdded();
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}
	return (
		<section className="panel add-panel" aria-labelledby="add-title">
			<div className="section-heading">
				<h2 id="add-title">머신 추가</h2>
				<span className="muted small">
					연결 확인 후 첫 인벤토리를 수집합니다.
				</span>
			</div>
			<form onSubmit={(event) => void submit(event)}>
				<fieldset disabled={busy} className="form-grid">
					<label>
						SSH 별칭 <span className="required">필수</span>
						<input
							name="alias"
							required
							autoComplete="off"
							placeholder="nima-studio"
						/>
					</label>
					<label>
						표시 이름
						<input name="name" autoComplete="off" placeholder="별칭과 같게" />
					</label>
					<label>
						전송 방식
						<select name="transport" defaultValue="ssh">
							<option value="ssh">ssh · 원격 머신</option>
							<option value="local">local · 이 허브</option>
						</select>
					</label>
					<label>
						플랫폼
						<select name="platform" defaultValue="auto">
							<option value="auto">auto · 자동 판별</option>
							<option value="posix">posix · macOS / Linux</option>
							<option value="windows">windows</option>
						</select>
					</label>
				</fieldset>
				<ErrorNotice message={error} />
				<div className="form-footer">
					<p className="muted small">
						~/.ssh/config의 별칭을 사용합니다. 머신에 별도 설치는 필요 없습니다.
					</p>
					<div className="actions">
						<button type="button" onClick={onCancel} disabled={busy}>
							취소
						</button>
						<button type="submit" className="primary" disabled={busy}>
							{busy ? <Spinner /> : null}
							{busy ? "연결 확인 · 수집 중…" : "추가하고 수집"}
						</button>
					</div>
				</div>
			</form>
		</section>
	);
}
