import type { FormEvent } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type {
	DesiredCandidate,
	DesiredEnsure,
	DesiredItem,
	DesiredManager,
	DesiredRow,
	DesiredStatus,
	DesiredView,
	MachinePlan,
} from "../shared/types";
import {
	api,
	Empty,
	ErrorNotice,
	errorMessage,
	Loading,
	PageHeading,
	Spinner,
	useResource,
} from "./ui";

export const MANAGER_LABELS: Record<DesiredManager, string> = {
	brew_formula: "brew formula",
	brew_cask: "brew cask",
	pacman: "pacman",
	mise: "mise",
};

const statusLabels: Record<DesiredStatus, { label: string; tone: string }> = {
	in_sync: { label: "맞음", tone: "online" },
	drifted: { label: "다름", tone: "warning" },
	pending: { label: "적용 중", tone: "collecting" },
	blocked: { label: "적용 불가", tone: "error" },
	unknown: { label: "알 수 없음", tone: "unknown" },
};

function DesiredStatusLabel({ status }: { status: DesiredStatus }) {
	const { label, tone } = statusLabels[status];
	return (
		<span className={`status ${tone}`}>
			{status === "pending" ? <Spinner /> : <span className="status-dot" />}
			{label}
		</span>
	);
}

const itemKey = (manager: DesiredManager, name: string) => `${manager}:${name}`;

/** Managed items by `${manager}:${name}`, for marking rows that are already adopted. */
export function useManagedItems() {
	const { data, refresh } = useResource<DesiredView>("/api/desired");
	const managed = useMemo(
		() =>
			new Map(
				(data?.items ?? []).map((item) => [
					itemKey(item.manager, item.name),
					item,
				]),
			),
		[data],
	);
	return { managed, refresh };
}

/** "채택" for an unmanaged item, or a link to the desired page when it is managed. */
export function AdoptButton({
	manager,
	name,
	managed,
	onChanged,
}: {
	manager: DesiredManager;
	name: string;
	managed: Map<string, DesiredItem>;
	onChanged: () => void;
}) {
	const [open, setOpen] = useState(false);
	if (managed.has(itemKey(manager, name)))
		return (
			<a className="managed-link small" href="#/desired">
				관리 중
			</a>
		);
	return (
		<>
			<button
				type="button"
				className="small-button quiet adopt-button"
				onClick={() => setOpen(true)}
				aria-label={`${name} 채택`}
			>
				채택
			</button>
			{/* Portal: the button sits in table cells whose font and alignment the dialog must not inherit. */}
			{open
				? createPortal(
						<DesiredDialog
							manager={manager}
							name={name}
							onClose={() => setOpen(false)}
							onSaved={() => {
								setOpen(false);
								onChanged();
							}}
						/>,
						document.body,
					)
				: null}
		</>
	);
}

/**
 * Adopt a new item or edit a managed one. Targets come from the hub's candidates: machines
 * that cannot apply this manager are shown but cannot be picked.
 */
export function DesiredDialog({
	manager,
	name,
	existing,
	onClose,
	onSaved,
}: {
	manager: DesiredManager;
	name: string;
	existing?: DesiredItem;
	onClose: () => void;
	onSaved: () => void;
}) {
	const ref = useRef<HTMLDialogElement>(null);
	const { data: candidates, error: loadError } = useResource<
		DesiredCandidate[]
	>(
		`/api/desired/candidates?manager=${manager}&name=${encodeURIComponent(name)}`,
	);
	const [ensure, setEnsure] = useState<DesiredEnsure>(
		existing?.ensure ?? "present",
	);
	const [version, setVersion] = useState(existing?.version ?? "");
	const [targets, setTargets] = useState<number[] | null>(
		existing?.machineIds ?? null,
	);
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");

	useEffect(() => {
		ref.current?.showModal();
	}, []);

	// New items start from the current state: machines that have it, and its global version.
	useEffect(() => {
		if (!candidates || existing) return;
		setTargets(
			candidates
				.filter((c) => !c.blocked && c.installed)
				.map((c) => c.machineId),
		);
		const current = candidates.find((c) => c.version)?.version;
		if (current) setVersion(current);
	}, [candidates, existing]);

	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setBusy(true);
		setError("");
		try {
			const body = {
				ensure,
				version: manager === "mise" ? version.trim() || null : null,
				machineIds: targets ?? [],
			};
			await (existing
				? api(`/api/desired/${existing.id}`, "PATCH", body)
				: api("/api/desired", "POST", { manager, name, ...body }));
			onSaved();
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}

	const toggle = (id: number) =>
		setTargets((current) =>
			current?.includes(id)
				? current.filter((x) => x !== id)
				: [...(current ?? []), id],
		);

	return (
		<dialog
			ref={ref}
			className="desired-dialog"
			aria-labelledby="desired-dialog-title"
			onClose={onClose}
		>
			<form onSubmit={(event) => void submit(event)}>
				<div className="section-heading">
					<div>
						<p className="eyebrow">{MANAGER_LABELS[manager]}</p>
						<h2 id="desired-dialog-title" className="mono">
							{name}
						</h2>
					</div>
					<button
						type="button"
						className="quiet small-button"
						onClick={() => ref.current?.close()}
					>
						닫기
					</button>
				</div>
				<fieldset disabled={busy} className="dialog-body">
					<div className="segmented" role="radiogroup" aria-label="원하는 상태">
						{(["present", "absent"] as const).map((value) => (
							<label
								key={value}
								className={`segment ${ensure === value ? "selected" : ""}`}
							>
								<input
									type="radio"
									name="ensure"
									value={value}
									checked={ensure === value}
									onChange={() => setEnsure(value)}
								/>
								{value === "present" ? "설치되어 있어야 함" : "없어야 함"}
							</label>
						))}
					</div>
					{manager === "mise" && ensure === "present" ? (
						<label>
							전역 버전
							<input
								className="mono"
								value={version}
								onChange={(event) => setVersion(event.target.value)}
								placeholder="latest, 22, 1.7.1"
								required
							/>
							<span className="muted small">
								~/.config/mise/config.toml에 요청할 버전입니다.
							</span>
						</label>
					) : null}
					<div className="dialog-targets">
						<span className="label-text">적용 대상 머신</span>
						{loadError ? <ErrorNotice message={loadError} /> : null}
						{!candidates ? (
							<p className="muted small" role="status">
								<Spinner /> 머신 상태를 확인하는 중…
							</p>
						) : (
							candidates.map((c) => (
								<label
									key={c.machineId}
									className={`exec-target ${c.blocked ? "blocked" : ""}`}
								>
									<input
										type="checkbox"
										checked={targets?.includes(c.machineId) ?? false}
										disabled={!!c.blocked}
										onChange={() => toggle(c.machineId)}
									/>
									<span className="exec-target-name">{c.machineName}</span>
									<span className="muted small">
										{c.blocked ??
											(c.installed
												? `설치됨${c.version ? ` · ${c.version}` : ""}`
												: "없음")}
									</span>
								</label>
							))
						)}
					</div>
				</fieldset>
				<ErrorNotice message={error} />
				<div className="form-footer">
					<p className="muted small">
						저장만 하고 설치·제거는 맞추기 화면에서 확인 후 적용합니다.
					</p>
					<button
						type="submit"
						className="primary"
						disabled={busy || !candidates}
					>
						{busy ? <Spinner /> : null}
						{existing ? "변경 저장" : "채택"}
					</button>
				</div>
			</form>
		</dialog>
	);
}

function stepLabel(step: MachinePlan["steps"][number]) {
	const target =
		step.version && step.action === "install"
			? `${step.name}@${step.version}`
			: step.name;
	return `${step.action === "install" ? "설치" : "제거"} · ${MANAGER_LABELS[step.manager]} ${target}`;
}

function PlanPanel({
	plans,
	onApplied,
}: {
	plans: MachinePlan[];
	onApplied: () => void;
}) {
	const [selected, setSelected] = useState<number[]>(() =>
		plans.map((p) => p.machineId),
	);
	const [confirming, setConfirming] = useState(false);
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const chosen = plans.filter((p) => selected.includes(p.machineId));

	async function apply() {
		setBusy(true);
		setError("");
		try {
			const { id } = await api<{ id: number }>("/api/desired/apply", "POST", {
				plans: chosen.map((p) => ({
					machineId: p.machineId,
					command: p.command,
				})),
			});
			window.location.hash = `#/exec/${id}`;
		} catch (cause) {
			setError(errorMessage(cause));
			setConfirming(false);
			onApplied();
		} finally {
			setBusy(false);
		}
	}

	return (
		<section className="panel" aria-labelledby="plan-title">
			<div className="section-heading">
				<div className="row-title">
					<h2 id="plan-title">계획</h2>
					<span className="count-label mono">{plans.length}</span>
				</div>
				<span className="muted small">
					다른 항목을 맞추는 명령입니다. 확인한 명령 그대로 실행됩니다.
				</span>
			</div>
			{plans.length === 0 ? (
				<Empty title="모든 대상 머신이 원하는 상태와 같습니다." />
			) : (
				<>
					<ul className="plan-list">
						{plans.map((plan) => (
							<li key={plan.machineId} className="plan-machine">
								<label className="checkbox-label">
									<input
										type="checkbox"
										checked={selected.includes(plan.machineId)}
										disabled={confirming || busy}
										onChange={() =>
											setSelected((current) =>
												current.includes(plan.machineId)
													? current.filter((id) => id !== plan.machineId)
													: [...current, plan.machineId],
											)
										}
									/>
									<strong>{plan.machineName}</strong>
									<span className="muted small">{plan.steps.length}건</span>
								</label>
								<ul className="plan-steps">
									{plan.steps.map((step) => (
										<li
											key={step.itemId}
											className={`plan-step ${step.action}`}
										>
											{stepLabel(step)}
										</li>
									))}
								</ul>
								<details>
									<summary className="small">실행할 명령</summary>
									<pre className="exec-command">{plan.command}</pre>
								</details>
							</li>
						))}
					</ul>
					<ErrorNotice message={error} />
					{confirming ? (
						<section
							className="exec-confirm"
							aria-labelledby="apply-confirm-title"
						>
							<h3 id="apply-confirm-title">
								{chosen.length}대에서 패키지를 설치·제거합니다
							</h3>
							<ul className="target-chips">
								{chosen.map((p) => (
									<li key={p.machineId} className="mono">
										{p.machineName} · {p.steps.length}건
									</li>
								))}
							</ul>
							<div className="form-footer exec-confirm-footer">
								<p className="muted small">
									머신마다 제한 시간 30분. 끝나면 바로 다시 수집해 상태를
									확인합니다.
								</p>
								<div className="actions">
									<button
										type="button"
										onClick={() => setConfirming(false)}
										disabled={busy}
									>
										취소
									</button>
									<button
										type="button"
										className="primary"
										onClick={() => void apply()}
										disabled={busy}
									>
										{busy ? <Spinner /> : null}
										{chosen.length}대에 적용
									</button>
								</div>
							</div>
						</section>
					) : (
						<div className="form-footer">
							<p className="muted small">
								적용은 실행 기록에 남고, 실행 화면에서 출력을 실시간으로 봅니다.
							</p>
							<button
								type="button"
								className="primary"
								disabled={chosen.length === 0}
								onClick={() => setConfirming(true)}
							>
								적용 확인
							</button>
						</div>
					)}
				</>
			)}
		</section>
	);
}

function AddItem({
	onAdd,
}: {
	onAdd: (manager: DesiredManager, name: string) => void;
}) {
	const [manager, setManager] = useState<DesiredManager>("brew_formula");
	const [name, setName] = useState("");
	return (
		<form
			className="add-item"
			onSubmit={(event) => {
				event.preventDefault();
				if (name.trim()) onAdd(manager, name.trim());
			}}
		>
			<label>
				<span className="sr-only">관리 대상</span>
				<select
					value={manager}
					onChange={(event) => setManager(event.target.value as DesiredManager)}
				>
					{Object.entries(MANAGER_LABELS).map(([value, label]) => (
						<option key={value} value={value}>
							{label}
						</option>
					))}
				</select>
			</label>
			<label>
				<span className="sr-only">이름</span>
				<input
					className="mono"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder="패키지 또는 도구 이름"
					autoComplete="off"
				/>
			</label>
			<button type="submit" disabled={!name.trim()}>
				항목 추가
			</button>
		</form>
	);
}

export function DesiredPage() {
	const view = useResource<DesiredView>("/api/desired", 5000);
	const plan = useResource<MachinePlan[]>("/api/desired/plan", 5000);
	const [dialog, setDialog] = useState<{
		manager: DesiredManager;
		name: string;
		existing?: DesiredItem;
	} | null>(null);
	const [error, setError] = useState("");
	const refresh = () => {
		void view.refresh();
		void plan.refresh();
	};

	async function remove(item: DesiredRow) {
		if (
			!window.confirm(
				`${item.name}을(를) 관리에서 뺄까요? 머신의 패키지는 그대로 둡니다.`,
			)
		)
			return;
		try {
			await api(`/api/desired/${item.id}`, "DELETE");
			refresh();
		} catch (cause) {
			setError(errorMessage(cause));
		}
	}

	const targetMachines =
		view.data?.machines.filter((m) =>
			view.data?.items.some((item) => item.machineIds.includes(m.id)),
		) ?? [];
	// Remount the plan when it changes so the machine selection follows the new plan.
	const planKey = (plan.data ?? []).map((p) => p.command).join("\u0000");

	return (
		<>
			<PageHeading
				eyebrow="DESIRED STATE"
				title="맞추기"
				description="채택한 패키지·도구를 머신마다 원하는 상태로 맞춥니다. 허브는 차이만 계산하고, 적용은 확인한 뒤에만 합니다."
			/>
			<ErrorNotice
				message={error || view.error || plan.error}
				retry={refresh}
			/>
			{plan.loading ? (
				<Loading />
			) : plan.data ? (
				<PlanPanel key={planKey} plans={plan.data} onApplied={refresh} />
			) : null}
			<section className="panel" aria-labelledby="desired-items-title">
				<div className="section-heading">
					<div className="row-title">
						<h2 id="desired-items-title">관리 항목</h2>
						<span className="count-label mono">
							{view.data?.items.length ?? 0}
						</span>
					</div>
					<AddItem onAdd={(manager, name) => setDialog({ manager, name })} />
				</div>
				{view.loading ? (
					<Loading />
				) : view.data?.items.length ? (
					<div className="table-scroll inventory-scroll">
						<table className="matrix desired-matrix">
							<thead>
								<tr>
									<th scope="col">항목</th>
									<th scope="col">원하는 상태</th>
									{targetMachines.map((m) => (
										<th key={m.id} scope="col">
											<a href={`#/machines/${m.id}`}>{m.name}</a>
										</th>
									))}
									<th scope="col">
										<span className="sr-only">동작</span>
									</th>
								</tr>
							</thead>
							<tbody>
								{view.data.items.map((item) => (
									<tr key={item.id}>
										<th scope="row">
											<span className="mono">{item.name}</span>
											<span className="muted small item-manager">
												{MANAGER_LABELS[item.manager]}
											</span>
										</th>
										<td className="nowrap">
											{item.ensure === "present"
												? `설치${item.version ? ` · ${item.version}` : ""}`
												: "없음"}
										</td>
										{targetMachines.map((m) => {
											const cell = item.cells[m.id];
											return (
												<td key={m.id} className={cell ? "" : "not-applicable"}>
													{cell ? (
														<>
															<DesiredStatusLabel status={cell.status} />
															{cell.detail ? (
																<span className="cell-detail muted small">
																	{cell.detail}
																</span>
															) : null}
														</>
													) : (
														<span className="small">대상 아님</span>
													)}
												</td>
											);
										})}
										<td>
											<div className="actions row-actions">
												<button
													type="button"
													className="small-button"
													onClick={() =>
														setDialog({
															manager: item.manager,
															name: item.name,
															existing: item,
														})
													}
													aria-label={`${MANAGER_LABELS[item.manager]} ${item.name} 편집`}
												>
													편집
												</button>
												<button
													type="button"
													className="small-button danger"
													onClick={() => void remove(item)}
													aria-label={`${item.name} 관리 해제`}
												>
													해제
												</button>
											</div>
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				) : (
					<Empty title="관리 중인 항목이 없습니다.">
						<p>
							<a href="#/compare">비교</a> 화면이나 머신 인벤토리에서 “채택”을
							누르거나, 위에서 직접 추가하세요.
						</p>
					</Empty>
				)}
			</section>
			{dialog ? (
				<DesiredDialog
					key={`${dialog.manager}:${dialog.name}`}
					manager={dialog.manager}
					name={dialog.name}
					existing={dialog.existing}
					onClose={() => setDialog(null)}
					onSaved={() => {
						setDialog(null);
						refresh();
					}}
				/>
			) : null}
		</>
	);
}
