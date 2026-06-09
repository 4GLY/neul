import type { ReactElement } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { DashboardData, MachineEvent } from "./api";
import {
	loadDashboardData,
	loadMachineEvents,
	OwnerSessionRequiredError,
	repairDrift,
} from "./api";
import { DashboardWorkspace } from "./DashboardWorkspace";
import {
	selectDashboardMachineId,
	shouldPreserveEventsOnMachineTransition,
} from "./dashboardView";
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
	const eventRequestId = useRef(0);

	const showSetup = useCallback((): void => {
		setDashboard(null);
		setSelectedMachineId("");
		setEvents([]);
		setActivityNotice("");
		setOnboardingOpen(false);
		setEditorOpen(false);
		setLoadState("setup");
	}, []);

	const refreshDashboard = useCallback(async (): Promise<void> => {
		setLoadState((current) => (current === "ready" ? "ready" : "loading"));
		try {
			const data = await loadDashboardData();
			setDashboard(data);
			setSelectedMachineId((current) =>
				selectDashboardMachineId(current, data.machines),
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

	const machines = dashboard?.machines ?? [];
	const fallbackMachine = machines[0];
	const selectedMachine =
		machines.find((machine) => machine.id === selectedMachineId) ??
		fallbackMachine;
	const eventMachineId = selectedMachine?.id ?? "";
	const previousEventMachineId = useRef<string | null>(null);
	const activeEventMachineId = useRef(eventMachineId);

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
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				showSetup();
				return;
			}
			setActivityNotice("로그를 불러오지 못했습니다");
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
