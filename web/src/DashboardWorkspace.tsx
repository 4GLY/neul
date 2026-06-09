import type { ReactElement } from "react";
import { useMemo } from "react";
import type { DashboardData, MachineEvent } from "./api";
import { copy } from "./copy";
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

type DashboardWorkspaceProps = {
	readonly activeView: "dashboard" | "ledger";
	readonly activityNotice: string;
	readonly dashboard: DashboardData | null;
	readonly editorOpen: boolean;
	readonly events: readonly MachineEvent[];
	readonly loadState: "loading" | "ready" | "error";
	readonly onboardingOpen: boolean;
	readonly osFilter: "all" | Machine["os"];
	readonly runState: "idle" | "running";
	readonly selectedMachineId: string;
	readonly statusFilter: "all" | MachineStatus;
	readonly onConnected: () => void;
	readonly onEditorToggle: () => void;
	readonly onOwnerSessionRequired: () => void;
	readonly onOnboardingClose: () => void;
	readonly onOnboardingOpen: () => void;
	readonly onOpenLogs: () => void;
	readonly onOsFilterChange: (next: "all" | Machine["os"]) => void;
	readonly onRepairDrift: () => void;
	readonly onResourceSaved: () => void;
	readonly onReconcile: () => void;
	readonly onSelectedMachineChange: (next: string) => void;
	readonly onStatusFilterChange: (next: "all" | MachineStatus) => void;
	readonly onViewToggle: () => void;
};

export function DashboardWorkspace({
	activeView,
	activityNotice,
	dashboard,
	editorOpen,
	events,
	loadState,
	onboardingOpen,
	osFilter,
	runState,
	selectedMachineId,
	statusFilter,
	onConnected,
	onEditorToggle,
	onOwnerSessionRequired,
	onOnboardingClose,
	onOnboardingOpen,
	onOpenLogs,
	onOsFilterChange,
	onRepairDrift,
	onResourceSaved,
	onReconcile,
	onSelectedMachineChange,
	onStatusFilterChange,
	onViewToggle,
}: DashboardWorkspaceProps): ReactElement {
	const machines = dashboard?.machines ?? [];
	const resources = dashboard?.resources ?? [];
	const activities = dashboard?.activities ?? [];
	const fallbackMachine = machines[0];
	const selectedMachine =
		machines.find((machine) => machine.id === selectedMachineId) ??
		fallbackMachine;
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

	const healthyCount = machines.filter(
		(machine) => machine.status === "healthy",
	).length;
	const driftedCount = machines.filter(
		(machine) => machine.status === "drifted",
	).length;
	const pendingCount = machines.filter(
		(machine) => machine.status === "pending",
	).length;
	const onlineCount = machines.filter(
		(machine) => machine.status !== "offline",
	).length;

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
									onClick={onEditorToggle}
								>
									Edit desired state
								</button>
								<button
									className="secondary-button"
									type="button"
									onClick={onViewToggle}
								>
									{activeView === "dashboard"
										? copy.dashboard.showLedger
										: copy.dashboard.showDashboard}
								</button>
								<button
									className="primary-button"
									type="button"
									onClick={onReconcile}
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
						<MetricStrip
							healthyCount={healthyCount}
							driftedCount={driftedCount}
							pendingCount={pendingCount}
							machineCount={machines.length}
							onlineCount={onlineCount}
						/>
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
							/>
						) : null}
						{loadState === "ready" && machines.length === 0 ? (
							<StatePanel
								title={copy.dashboard.emptyState.title}
								body={copy.dashboard.emptyState.body}
								action={copy.dashboard.emptyState.action}
								onAction={onOnboardingOpen}
							/>
						) : null}
						{onboardingOpen ? (
							<OnboardingWizard
								onClose={onOnboardingClose}
								onConnected={onConnected}
								onOwnerSessionRequired={onOwnerSessionRequired}
							/>
						) : null}
						{editorOpen ? (
							<ResourceEditor
								onOwnerSessionRequired={onOwnerSessionRequired}
								onSaved={onResourceSaved}
							/>
						) : null}
						{loadState === "ready" &&
						selectedMachine !== undefined &&
						activeView === "ledger" ? (
							<DesiredLivePreview
								resources={resources}
								selectedMachine={selectedMachine}
							/>
						) : null}
						{loadState === "ready" && machines.length > 0 ? (
							<MachineFilters
								statusFilter={statusFilter}
								osFilter={osFilter}
								onStatusChange={onStatusFilterChange}
								onOsChange={onOsFilterChange}
							/>
						) : null}
						{loadState === "ready" && machines.length > 0 ? (
							<MachineTable
								machines={visibleMachines}
								selectedMachineId={selectedMachineId}
								onSelect={onSelectedMachineChange}
							/>
						) : null}
						{loadState === "ready" &&
						selectedMachine !== undefined &&
						activeView === "dashboard" ? (
							<DesiredLivePreview
								resources={resources}
								selectedMachine={selectedMachine}
							/>
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
								onRepairDrift={onRepairDrift}
								onOpenLogs={onOpenLogs}
							/>
						)}
						<ActivityFeed activities={activities} />
					</aside>
				</div>
			</section>
		</main>
	);
}
