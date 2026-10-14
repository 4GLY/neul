import type { FormEvent, ReactNode } from "react";
import { useDeferredValue, useState } from "react";
import type { Inventory, MachineDetail, Platform } from "../shared/types";
import {
	api,
	bytes,
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

const tabs = [
	{ id: "brewFormulae", title: "brew formulae" },
	{ id: "brewCasks", title: "casks" },
	{ id: "pacmanExplicit", title: "pacman" },
	{ id: "mise", title: "mise" },
	{ id: "files", title: "추적 파일" },
] as const;
type InventoryTab = (typeof tabs)[number]["id"];

export function Detail({ id }: { id: number }) {
	const {
		data: machine,
		error,
		loading,
		refresh,
	} = useResource<MachineDetail>(`/api/machines/${id}`, 15000);
	const [busy, setBusy] = useState(false);
	const [editing, setEditing] = useState(false);
	const [actionError, setActionError] = useState("");
	const [search, setSearch] = useState("");
	const query = useDeferredValue(search.trim().toLowerCase());
	// null until the user picks a tab: default to the first category this machine reports.
	const [pickedTab, setTab] = useState<InventoryTab | null>(null);
	async function collect() {
		setBusy(true);
		setActionError("");
		try {
			await api(`/api/machines/${id}/collect`, "POST");
		} catch (cause) {
			setActionError(errorMessage(cause));
		} finally {
			setBusy(false);
			await refresh();
		}
	}
	async function remove() {
		if (
			!machine ||
			!window.confirm(
				`“${machine.name}” 머신과 모든 수집 기록을 삭제할까요? 이 작업은 되돌릴 수 없습니다.`,
			)
		)
			return;
		setBusy(true);
		setActionError("");
		try {
			await api(`/api/machines/${id}`, "DELETE");
			window.location.hash = "/";
		} catch (cause) {
			setActionError(errorMessage(cause));
			setBusy(false);
		}
	}
	if (loading) return <Loading />;
	if (!machine)
		return (
			<>
				<a className="back-link" href="#/">
					← 머신 목록
				</a>
				<ErrorNotice message={error} retry={() => void refresh()} />
			</>
		);
	const facts = machine.facts;
	const tab =
		pickedTab ??
		tabs.find((item) => machine.inventory?.[item.id] != null)?.id ??
		"brewFormulae";
	return (
		<>
			<a className="back-link" href="#/">
				← 머신 목록
			</a>
			<PageHeading
				eyebrow={`${machine.transport.toUpperCase()} / ${machine.alias}`}
				title={machine.name}
				description={`${machine.platform} · ${machine.enabled ? "주기 수집 활성" : "주기 수집 일시 중지"}`}
			>
				<Status
					status={machine.status}
					collecting={busy || machine.collecting}
				/>
				<button
					type="button"
					onClick={() => void collect()}
					disabled={busy || machine.collecting}
				>
					{busy || machine.collecting ? <Spinner /> : null}수집
				</button>
				<button
					type="button"
					onClick={() => setEditing(!editing)}
					aria-expanded={editing}
				>
					머신 편집
				</button>
			</PageHeading>
			<ErrorNotice message={error} retry={() => void refresh()} />
			<ErrorNotice message={actionError} />
			{editing ? (
				<EditMachine
					key={machine.id}
					machine={machine}
					onSaved={() => {
						setEditing(false);
						void refresh();
					}}
					onDelete={() => void remove()}
					disabled={busy}
				/>
			) : null}
			<section className="panel facts-panel" aria-label="머신 정보">
				<div className="section-heading">
					<h2>시스템</h2>
					<time
						className="muted small"
						dateTime={machine.lastSeenAt ?? undefined}
					>
						정상 수집 {relativeTime(machine.lastSeenAt)}
					</time>
				</div>
				{facts ? (
					<>
						<dl className="facts-grid">
							<Fact label="운영체제">{facts.osName ?? facts.os}</Fact>
							<Fact label="호스트">{facts.hostname}</Fact>
							<Fact label="모델">{facts.model}</Fact>
							<Fact label="아키텍처 / CPU">
								{facts.arch ?? "—"} / {facts.cpuCount ?? "—"} cores
							</Fact>
							<Fact label="커널">{facts.kernel}</Fact>
							<Fact label="사용자">{facts.user}</Fact>
							<Fact label="가동 시간">{uptime(facts.uptimeS)}</Fact>
							<Fact label="부하 · 1 / 5 / 15분">
								{facts.load?.map((value) => value.toFixed(2)).join(" / ")}
							</Fact>
							<Fact label="배터리">
								{facts.battery
									? `${facts.battery.percent}% · ${facts.battery.state}`
									: "—"}
							</Fact>
							<Fact label="업데이트 대기">
								{[
									facts.updates.brew
										? `brew ${facts.updates.brew.length}`
										: null,
									facts.updates.pacman
										? `pacman ${facts.updates.pacman.length}`
										: null,
								]
									.filter(Boolean)
									.join(" · ")}
							</Fact>
							<Fact label="감시 저장소">
								{facts.repos?.length
									? facts.repos.map((repo) => (
											<span key={repo.path} className="fact-line">
												{repo.path}{" "}
												{repo.state === "missing"
													? "· 없음"
													: `· 변경 ${repo.dirty ?? "—"} · push 대기 ${repo.ahead ?? "—"}`}
											</span>
										))
									: null}
							</Fact>
						</dl>
						<div className="resource-details">
							<div>
								<Meter label="메모리" value={memoryPercent(facts)} />
								<p className="muted small mono">
									전체 {bytes(facts.memTotalB)} · 사용 가능{" "}
									{bytes(facts.memAvailableB)}
								</p>
							</div>
							<div>
								<Meter
									label={`디스크 · ${facts.disk?.path ?? "—"}`}
									value={diskPercent(facts)}
								/>
								<p className="muted small mono">
									사용 {bytes(facts.disk?.usedB)} / {bytes(facts.disk?.totalB)}{" "}
									· 사용 가능 {bytes(facts.disk?.availableB)}
								</p>
							</div>
						</div>
					</>
				) : (
					<Empty title="아직 정상 수집된 시스템 정보가 없습니다." />
				)}
			</section>
			<section
				className="panel inventory-panel"
				aria-labelledby="inventory-title"
			>
				<div className="section-heading">
					<div>
						<h2 id="inventory-title">인벤토리</h2>
						{machine.inventorySince ? (
							<p className="muted small">
								현재 구성 ·{" "}
								{new Date(machine.inventorySince).toLocaleString("ko-KR")}부터
							</p>
						) : null}
					</div>
					<label className="search-label">
						<span className="sr-only">인벤토리 검색</span>
						<input
							type="search"
							placeholder="이름, 버전, 경로 검색…"
							value={search}
							onChange={(event) => setSearch(event.target.value)}
						/>
					</label>
				</div>
				<fieldset className="tabs">
					<legend className="sr-only">인벤토리 종류</legend>
					{tabs.map((item) => (
						<button
							type="button"
							key={item.id}
							className={tab === item.id ? "active" : ""}
							aria-pressed={tab === item.id}
							onClick={() => setTab(item.id)}
						>
							{item.title}
							<span className="tab-count mono">
								{machine.inventory?.[item.id]?.length ?? "—"}
							</span>
						</button>
					))}
				</fieldset>
				{machine.inventory ? (
					<InventoryTable
						inventory={machine.inventory}
						tab={tab}
						query={query}
					/>
				) : (
					<Empty title="수집된 인벤토리가 없습니다." />
				)}
			</section>
			<section className="panel" aria-labelledby="runs-title">
				<div className="section-heading">
					<h2 id="runs-title">최근 실행</h2>
					<span className="muted small">
						최근 {machine.recentRuns.length}건
					</span>
				</div>
				{machine.recentRuns.length ? (
					<div className="table-scroll runs-scroll">
						<table>
							<thead>
								<tr>
									<th scope="col">수집 시각</th>
									<th scope="col">결과</th>
									<th scope="col">소요 시간</th>
									<th scope="col">인벤토리</th>
									<th scope="col">오류</th>
								</tr>
							</thead>
							<tbody>
								{machine.recentRuns.map((run) => (
									<tr key={run.id}>
										<td>
											<time dateTime={run.startedAt}>
												{new Date(run.startedAt).toLocaleString("ko-KR")}
											</time>
										</td>
										<td>
											<Status
												status={run.status === "ok" ? "online" : run.status}
											/>
										</td>
										<td className="mono">
											{(run.durationMs / 1000).toFixed(1)}s
										</td>
										<td>
											{run.inventoryChanged ? (
												<span className="changed-label">변경됨</span>
											) : (
												"변경 없음"
											)}
										</td>
										<td className="run-error">{run.error ?? "—"}</td>
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

function Fact({ label, children }: { label: string; children: ReactNode }) {
	return (
		<div>
			<dt>{label}</dt>
			<dd>{children || "—"}</dd>
		</div>
	);
}

function EditMachine({
	machine,
	onSaved,
	onDelete,
	disabled,
}: {
	machine: MachineDetail;
	onSaved: () => void;
	onDelete: () => void;
	disabled: boolean;
}) {
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		const values = new FormData(event.currentTarget);
		setBusy(true);
		setError("");
		try {
			await api(`/api/machines/${machine.id}`, "PATCH", {
				name: String(values.get("name")).trim(),
				platform: values.get("platform") as Platform,
				enabled: values.get("enabled") === "on",
				notifyUnreachable: values.get("notifyUnreachable") === "on",
			});
			onSaved();
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}
	return (
		<section className="panel">
			<div className="section-heading">
				<h2>머신 편집</h2>
				<span className="muted small mono">
					{machine.alias} · {machine.transport}
				</span>
			</div>
			<form onSubmit={(event) => void submit(event)}>
				<fieldset className="edit-grid" disabled={busy || disabled}>
					<label>
						표시 이름
						<input name="name" defaultValue={machine.name} required />
					</label>
					<label>
						플랫폼
						<select name="platform" defaultValue={machine.platform}>
							<option value="posix">posix · macOS / Linux</option>
							<option value="windows">windows</option>
						</select>
					</label>
					<label className="checkbox-label">
						<input
							type="checkbox"
							name="enabled"
							defaultChecked={machine.enabled}
						/>
						주기 수집 활성
					</label>
					<label className="checkbox-label">
						<input
							type="checkbox"
							name="notifyUnreachable"
							defaultChecked={machine.notifyUnreachable}
						/>
						연결 끊김 알림 (잠자는 노트북은 끄기)
					</label>
				</fieldset>
				<ErrorNotice message={error} />
				<div className="form-footer">
					<button
						type="button"
						className="danger"
						onClick={onDelete}
						disabled={busy || disabled}
					>
						머신 삭제
					</button>
					<button type="submit" className="primary" disabled={busy || disabled}>
						{busy ? <Spinner /> : null}변경 저장
					</button>
				</div>
			</form>
		</section>
	);
}

function InventoryTable({
	inventory,
	tab,
	query,
}: {
	inventory: Inventory;
	tab: InventoryTab;
	query: string;
}) {
	const source = inventory[tab];
	if (source === null)
		return <Empty title="이 머신에서 수집하지 않는 항목입니다." />;
	const headers =
		tab === "files"
			? ["경로", "종류", "SHA-256", "크기", "링크 대상"]
			: tab === "mise"
				? ["도구", "활성 버전", "요청 버전", "설치 버전", "설정 원본"]
				: tab === "pacmanExplicit"
					? ["패키지", "버전"]
					: ["패키지", "설치 버전", "설치 방식"];
	let rows: { key: string; search: string; cells: ReactNode[] }[];
	if (tab === "files")
		rows = (inventory.files ?? []).map((file) => ({
			key: file.path,
			search: `${file.path} ${file.kind} ${file.sha256 ?? ""} ${file.linkTarget ?? ""}`,
			cells: [
				file.path,
				<span key="kind" className={`file-kind ${file.kind}`}>
					{file.kind}
				</span>,
				<span key="hash" title={file.sha256 ?? undefined}>
					{file.sha256?.slice(0, 12) ?? "—"}
				</span>,
				bytes(file.size),
				file.linkTarget ?? "—",
			],
		}));
	else if (tab === "mise")
		rows = (inventory.mise ?? []).map((tool) => ({
			key: tool.name,
			search: `${tool.name} ${tool.activeVersion ?? ""} ${tool.requestedVersion ?? ""} ${tool.installedVersions.join(" ")} ${tool.source ?? ""}`,
			cells: [
				tool.name,
				<span key="active" className="active-version">
					{tool.activeVersion ?? "—"}
				</span>,
				tool.requestedVersion ?? "—",
				tool.installedVersions.join(", ") || "—",
				tool.source ?? "—",
			],
		}));
	else if (tab === "pacmanExplicit")
		rows = (inventory.pacmanExplicit ?? []).map((pkg) => ({
			key: pkg.name,
			search: `${pkg.name} ${pkg.version}`,
			cells: [pkg.name, pkg.version],
		}));
	else
		rows = (inventory[tab] ?? []).map((pkg) => ({
			key: pkg.name,
			search: `${pkg.name} ${pkg.versions.join(" ")}`,
			cells: [
				pkg.name,
				pkg.versions.join(", "),
				pkg.onRequest ? (
					<span key="request" className="requested-label">
						직접 설치
					</span>
				) : (
					<span key="dependency" className="muted">
						의존성
					</span>
				),
			],
		}));
	const visible = query
		? rows.filter((row) => row.search.toLowerCase().includes(query))
		: rows;
	return (
		<>
			<div className="table-meta muted small">
				{visible.length}개{query ? ` / 전체 ${rows.length}개` : ""}
				{tab === "pacmanExplicit"
					? ` · 직접 설치 패키지 (전체 ${inventory.pacmanTotal ?? "—"}개)`
					: ""}
			</div>
			{visible.length ? (
				<div className="table-scroll inventory-scroll">
					<table className="inventory-table">
						<thead>
							<tr>
								{headers.map((header) => (
									<th scope="col" key={header}>
										{header}
									</th>
								))}
							</tr>
						</thead>
						<tbody>
							{visible.map((row) => (
								<tr key={row.key}>
									{row.cells.map((cell, index) => (
										<td
											key={headers[index]}
											className={index === 0 ? "row-key mono" : "mono"}
										>
											{cell}
										</td>
									))}
								</tr>
							))}
						</tbody>
					</table>
				</div>
			) : (
				<Empty
					title={query ? "검색 결과가 없습니다." : "수집된 항목이 없습니다."}
				/>
			)}
		</>
	);
}
