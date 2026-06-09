import type { ReactElement } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { DashboardData, MachineEvent } from "./api";
import { loadDashboardData, loadMachineEvents, repairDrift } from "./api";
import { copy } from "./copy";
import {
	latestReconcileLabel,
	selectDashboardMachineId,
} from "./dashboardView";
import {
	DesiredLivePreview,
	MachineFilters,
	MachineTable,
	MetricStrip,
} from "./FleetPanels";
import { Sidebar, Topbar } from "./Layout";
import { OnboardingWizard } from "./OnboardingWizard";
import { ResourceEditor } from "./ResourceEditor";
import { ActivityFeed, MachineInspector } from "./SidePanel";
import { StatePanel } from "./StatePanel";
import type { Machine, MachineStatus } from "./types";

export function App(): ReactElement {
	const [dashboard, setDashboard] = useState<DashboardData | null>(null);
	const [loadState, setLoadState] = useState<"loading" | "ready" | "error">(
		"loading",
	);
	const [selectedMachineId, setSelectedMachineId] = useState("");
	const [statusFilter, setStatusFilter] = useState<"all" | MachineStatus>(
		"all",
	);
	const [osFilter, setOsFilter] = useState<"all" | Machine["os"]>("all");
	const [activeView, setActiveView] = useState<"dashboard" | "ledger">(
		"dashboard",
	);
	const [editorOpen, setEditorOpen] = useState(false);
	const [runState, setRunState] = useState<"idle" | "running">("idle");
	const [events, setEvents] = useState<readonly MachineEvent[]>([]);
	const [activityNotice, setActivityNotice] = useState("");
	const [onboardingOpen, setOnboardingOpen] = useState(false);
	const eventRequestId = useRef(0);

	const refreshDashboard = useCallback(async (): Promise<void> => {
		setLoadState((current) => (current === "ready" ? "ready" : "loading"));
		try {
			const data = await loadDashboardData();
			setDashboard(data);
			setSelectedMachineId((current) =>
				selectDashboardMachineId(current, data.machines),
			);
			setLoadState("ready");
		} catch {
			setLoadState("error");
		}
	}, []);

	useEffect(() => {
		void refreshDashboard();
	}, [refreshDashboard]);

	const machines = dashboard?.machines ?? [];
	const resources = dashboard?.resources ?? [];
	const activities = dashboard?.activities ?? [];
	const fallbackMachine = machines[0];
	const selectedMachine =
		machines.find((machine) => machine.id === selectedMachineId) ??
		fallbackMachine;
	const eventMachineId = selectedMachine?.id ?? "";
	const previousEventMachineId = useRef<string | null>(null);
	const activeEventMachineId = useRef(eventMachineId);
	const canRenderDashboard = dashboard !== null && loadState !== "loading";
	const canRenderFleet = canRenderDashboard && machines.length > 0;
	const canRenderDetails = canRenderDashboard && selectedMachine !== undefined;

	useEffect(() => {
		activeEventMachineId.current = eventMachineId;
		if (previousEventMachineId.current === eventMachineId) {
			return;
		}
		if (
			shouldPreserveEventsOnMachineTransition(previousEventMachineId.current)
		) {
			previousEventMachineId.current = eventMachineId;
			return;
		}
		previousEventMachineId.current = eventMachineId;
		setEvents([]);
	}, [eventMachineId]);

	const visibleMachines = useMemo(
		() =>
			machines.filter((machine) => {
				const matchesStatus =
					statusFilter === "all" || machine.status === statusFilter;
				const matchesOs = osFilter === "all" || machine.os === osFilter;
				return matchesStatus && matchesOs;
			}),
		[machines, osFilter, statusFilter],
	);

	function handleReconcile(): void {
		setRunState("running");
		window.setTimeout(() => setRunState("idle"), 1600);
	}

	async function handleRepairDrift(): Promise<void> {
		if (selectedMachine === undefined) {
			return;
		}
		try {
			await repairDrift(selectedMachine.id);
			setActivityNotice("복구 명령을 대기열에 추가했습니다");
			await refreshDashboard();
		} catch {
			setActivityNotice("복구 명령을 만들지 못했습니다");
		}
	}

	async function handleOpenLogs(): Promise<void> {
		if (selectedMachine === undefined) {
			return;
		}
		const machineID = selectedMachine.id;
		const requestId = eventRequestId.current + 1;
		eventRequestId.current = requestId;
		try {
			const nextEvents = await loadMachineEvents(machineID);
			if (
				eventRequestId.current !== requestId ||
				activeEventMachineId.current !== machineID
			) {
				return;
			}
			setEvents(nextEvents);
		} catch {
			setActivityNotice("로그를 불러오지 못했습니다");
		}
	}

	return (
		<main className="shell">
			<Sidebar />
			<section className="workspace">
				<Topbar runState={runState} />
				<div className="content-grid">
					<section className="main-column">
						<header className="page-header">
							<div>
								<p className="eyebrow">{copy.dashboard.pageEyebrow}</p>
								<h1>{copy.dashboard.pageTitle}</h1>
								<p className="subtle">{copy.dashboard.pageDescription}</p>
							</div>
							<div className="actions">
								<button
									className="secondary-button"
									type="button"
									onClick={() => setEditorOpen((current) => !current)}
								>
									Edit desired state
								</button>
								<button
									className="secondary-button"
									type="button"
									onClick={() =>
										setActiveView(
											activeView === "dashboard" ? "ledger" : "dashboard",
										)
									}
								>
									{activeView === "dashboard"
										? copy.dashboard.showLedger
										: copy.dashboard.showDashboard}
								</button>
								<button
									className="primary-button"
									type="button"
									onClick={handleReconcile}
								>
									{runState === "running"
										? copy.dashboard.reconciling
										: copy.dashboard.reconcileNow}
								</button>
							</div>
						</header>
						{activityNotice === "" ? null : (
							<StatePanel title="작업 대기열" body={activityNotice} />
						)}

						{canRenderFleet && dashboard !== null ? (
							<MetricStrip
								metrics={dashboard.metrics}
								latestReconcile={latestReconcileLabel(machines)}
							/>
						) : null}
						{loadState === "loading" ? (
							<StatePanel
								title="불러오는 중"
								body="대시보드를 불러오는 중입니다."
							/>
						) : null}
						{loadState === "error" ? (
							<StatePanel
								title="대시보드를 불러오지 못했습니다"
								body="서버 연결을 확인한 뒤 다시 시도하세요."
								action="다시 시도"
								onAction={() => {
									void refreshDashboard();
								}}
							/>
						) : null}
						{loadState === "ready" && machines.length === 0 ? (
							<StatePanel
								title={copy.dashboard.emptyState.title}
								body={copy.dashboard.emptyState.body}
								action={copy.dashboard.emptyState.action}
								onAction={() => setOnboardingOpen(true)}
							/>
						) : null}
						{onboardingOpen ? (
							<OnboardingWizard
								onClose={() => setOnboardingOpen(false)}
								onConnected={() => {
									setOnboardingOpen(false);
									void refreshDashboard();
								}}
							/>
						) : null}
						{editorOpen ? (
							<ResourceEditor
								onSaved={() => {
									void refreshDashboard();
								}}
							/>
						) : null}
						{canRenderDetails && activeView === "ledger" ? (
							<DesiredLivePreview resources={resources} />
						) : null}
						{canRenderFleet ? (
							<MachineFilters
								statusFilter={statusFilter}
								osFilter={osFilter}
								onStatusChange={setStatusFilter}
								onOsChange={setOsFilter}
							/>
						) : null}
						{canRenderFleet ? (
							<MachineTable
								machines={visibleMachines}
								selectedMachineId={selectedMachineId}
								onSelect={setSelectedMachineId}
							/>
						) : null}
						{canRenderDetails && activeView === "dashboard" ? (
							<DesiredLivePreview resources={resources} />
						) : null}
					</section>
					<aside className="side-panel">
						{selectedMachine === undefined ? (
							<StatePanel
								title="등록된 머신 없음"
								body="첫 머신이 연결되면 상태와 로그가 여기에 표시됩니다."
							/>
						) : (
							<MachineInspector
								machine={selectedMachine}
								events={events}
								onRepairDrift={() => {
									void handleRepairDrift();
								}}
								onOpenLogs={() => {
									void handleOpenLogs();
								}}
							/>
						)}
						<ActivityFeed activities={activities} />
					</aside>
				</div>
			</section>
		</main>
	);
}

function shouldPreserveEventsOnMachineTransition(
	previous: string | null,
): boolean {
	return previous === null || previous === "";
}
