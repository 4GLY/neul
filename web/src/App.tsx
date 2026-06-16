import type { ReactElement } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { DashboardData } from "./api";
import { loadDashboardData, OwnerSessionRequiredError } from "./api";
import { DashboardWorkspace } from "./DashboardWorkspace";
import {
	selectDashboardMachineId,
	shouldPreserveEventsOnMachineTransition,
} from "./dashboardView";
import { FirstRunSetup } from "./FirstRunSetup";
import { useRepairController } from "./repairController";
import type { Machine, MachineStatus } from "./types";

type LoadState = "loading" | "ready" | "error" | "setup";

export function App(): ReactElement {
	const [dashboard, setDashboard] = useState<DashboardData | null>(null);
	const [loadState, setLoadState] = useState<LoadState>("loading");
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
	const [onboardingOpen, setOnboardingOpen] = useState(false);
	const activeEventMachineId = useRef("");

	const showSetup = useCallback((): void => {
		setDashboard(null);
		setSelectedMachineId("");
		setOnboardingOpen(false);
		setEditorOpen(false);
		setLoadState("setup");
	}, []);

	const refreshDashboard =
		useCallback(async (): Promise<DashboardData | null> => {
			setLoadState((current) => (current === "ready" ? "ready" : "loading"));
			try {
				const data = await loadDashboardData();
				setDashboard(data);
				setSelectedMachineId((current) =>
					selectDashboardMachineId(current, data.machines),
				);
				setLoadState("ready");
				return data;
			} catch (error) {
				if (error instanceof OwnerSessionRequiredError) {
					showSetup();
					return null;
				}
				setLoadState("error");
				return null;
			}
		}, [showSetup]);

	useEffect(() => {
		void refreshDashboard();
	}, [refreshDashboard]);

	const machines = dashboard?.machines ?? [];
	const fallbackMachine = machines[0];
	const selectedMachine =
		machines.find((machine) => machine.id === selectedMachineId) ??
		fallbackMachine;
	const {
		activityNotice,
		clearRepairPoll,
		events,
		handleOpenLogs,
		handleRepairDrift,
		resetRepairState,
		selectedRepairResourceId,
		setSelectedRepairResourceId,
	} = useRepairController({
		selectedMachine,
		activeEventMachineId,
		refreshDashboard,
		showSetup,
	});
	const eventMachineId = selectedMachine?.id ?? "";
	const previousEventMachineId = useRef<string | null>(null);

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
		clearRepairPoll();
		resetRepairState();
	}, [clearRepairPoll, eventMachineId, resetRepairState]);

	function handleReconcile(): void {
		setRunState("running");
		window.setTimeout(() => setRunState("idle"), 1600);
	}

	if (loadState === "setup") {
		return (
			<FirstRunSetup
				onSetupComplete={async () => {
					await refreshDashboard();
				}}
			/>
		);
	}

	return (
		<DashboardWorkspace
			activeView={activeView}
			activityNotice={activityNotice}
			dashboard={dashboard}
			editorOpen={editorOpen}
			events={events}
			selectedRepairResourceId={selectedRepairResourceId}
			loadState={loadState}
			onConnected={() => {
				setOnboardingOpen(false);
				void refreshDashboard();
			}}
			onEditorToggle={() => setEditorOpen((current) => !current)}
			onOwnerSessionRequired={showSetup}
			onOnboardingClose={() => setOnboardingOpen(false)}
			onOnboardingOpen={() => setOnboardingOpen(true)}
			onOpenLogs={() => {
				void handleOpenLogs();
			}}
			onOsFilterChange={setOsFilter}
			onRepairDrift={() => {
				void handleRepairDrift();
			}}
			onRepairResourceSelect={setSelectedRepairResourceId}
			onResourceSaved={() => {
				void refreshDashboard();
			}}
			onReconcile={handleReconcile}
			onRetryLoad={() => {
				void refreshDashboard();
			}}
			onSelectedMachineChange={setSelectedMachineId}
			onStatusFilterChange={setStatusFilter}
			onViewToggle={() =>
				setActiveView(activeView === "dashboard" ? "ledger" : "dashboard")
			}
			onboardingOpen={onboardingOpen}
			osFilter={osFilter}
			runState={runState}
			selectedMachineId={selectedMachineId}
			statusFilter={statusFilter}
		/>
	);
}
