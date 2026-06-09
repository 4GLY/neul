import type { ReactElement } from "react";
import { useCallback, useEffect, useState } from "react";
import type { DashboardData, MachineEvent } from "./api";
import {
	loadDashboardData,
	loadMachineEvents,
	OwnerSessionRequiredError,
	repairDrift,
} from "./api";
import { DashboardWorkspace } from "./DashboardWorkspace";
import { FirstRunSetup } from "./FirstRunSetup";
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
	const [events, setEvents] = useState<readonly MachineEvent[]>([]);
	const [activityNotice, setActivityNotice] = useState("");
	const [onboardingOpen, setOnboardingOpen] = useState(false);

	const showSetup = useCallback((): void => {
		setDashboard(null);
		setSelectedMachineId("");
		setEvents([]);
		setActivityNotice("");
		setLoadState("setup");
	}, []);

	const refreshDashboard = useCallback(async (): Promise<void> => {
		setLoadState("loading");
		try {
			const data = await loadDashboardData();
			setDashboard(data);
			setSelectedMachineId(
				(current) => current || (data.machines[0]?.id ?? ""),
			);
			setLoadState("ready");
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				showSetup();
				return;
			}
			setLoadState("error");
		}
	}, [showSetup]);

	useEffect(() => {
		void refreshDashboard();
	}, [refreshDashboard]);

	const selectedMachine =
		dashboard?.machines.find((machine) => machine.id === selectedMachineId) ??
		dashboard?.machines[0];

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
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				showSetup();
				return;
			}
			throw error;
		}
	}

	async function handleOpenLogs(): Promise<void> {
		if (selectedMachine === undefined) {
			return;
		}
		try {
			const nextEvents = await loadMachineEvents(selectedMachine.id);
			setEvents(nextEvents);
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				showSetup();
				return;
			}
			throw error;
		}
	}

	if (loadState === "setup") {
		return <FirstRunSetup onSetupComplete={refreshDashboard} />;
	}

	return (
		<DashboardWorkspace
			activeView={activeView}
			activityNotice={activityNotice}
			dashboard={dashboard}
			editorOpen={editorOpen}
			events={events}
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
			onResourceSaved={() => {
				void refreshDashboard();
			}}
			onReconcile={handleReconcile}
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
