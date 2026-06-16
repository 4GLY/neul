import { type MutableRefObject, useCallback, useRef, useState } from "react";
import type { DashboardData, MachineEvent } from "./api";
import {
	loadMachineEvents,
	OwnerSessionRequiredError,
	repairDrift,
} from "./api";
import type { Machine } from "./types";

type RepairOutcome = "pending" | "terminal" | "stale";

const repairPollDelayMs = 5000;
const maxRepairPolls = 24;

type RepairControllerOptions = {
	readonly selectedMachine: Machine | undefined;
	readonly activeEventMachineId: MutableRefObject<string>;
	readonly refreshDashboard: () => Promise<DashboardData | null>;
	readonly showSetup: () => void;
};

export function useRepairController({
	selectedMachine,
	activeEventMachineId,
	refreshDashboard,
	showSetup,
}: RepairControllerOptions): {
	readonly activityNotice: string;
	readonly events: readonly MachineEvent[];
	readonly selectedRepairResourceId: string;
	readonly clearRepairPoll: () => void;
	readonly handleOpenLogs: () => Promise<void>;
	readonly handleRepairDrift: () => Promise<void>;
	readonly resetRepairState: () => void;
	readonly setSelectedRepairResourceId: (next: string) => void;
} {
	const [events, setEvents] = useState<readonly MachineEvent[]>([]);
	const [selectedRepairResourceId, setSelectedRepairResourceId] = useState("");
	const [activityNotice, setActivityNotice] = useState("");
	const eventRequestId = useRef(0);
	const repairRequestId = useRef(0);
	const repairPollTimeout = useRef<number | undefined>(undefined);

	const clearRepairPoll = useCallback((): void => {
		if (repairPollTimeout.current === undefined) {
			return;
		}
		window.clearTimeout(repairPollTimeout.current);
		repairPollTimeout.current = undefined;
	}, []);

	const resetRepairState = useCallback((): void => {
		clearRepairPoll();
		setEvents([]);
		setSelectedRepairResourceId("");
		setActivityNotice("");
	}, [clearRepairPoll]);

	async function handleRepairDrift(): Promise<void> {
		if (selectedMachine === undefined) {
			return;
		}
		try {
			const resourceIDs =
				selectedRepairResourceId === "" ? [] : [selectedRepairResourceId];
			const machineID = selectedMachine.id;
			const repairID = repairRequestId.current + 1;
			repairRequestId.current = repairID;
			clearRepairPoll();
			await repairDrift(machineID, resourceIDs);
			setActivityNotice("복구 명령 대기 중");
			const outcome = await refreshRepairOutcome(
				machineID,
				selectedRepairResourceId,
				repairID,
			);
			if (outcome === "pending") {
				scheduleRepairPoll(machineID, selectedRepairResourceId, repairID, 1);
			}
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
			setSelectedRepairResourceId(firstDriftedResourceId(nextEvents));
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				showSetup();
				return;
			}
			setActivityNotice("로그를 불러오지 못했습니다");
		}
	}

	async function refreshRepairOutcome(
		machineID: string,
		resourceID: string,
		repairID: number,
	): Promise<RepairOutcome> {
		const refreshed = await refreshDashboard();
		const refreshedEvents = await loadMachineEvents(machineID);
		if (
			repairRequestId.current !== repairID ||
			activeEventMachineId.current !== machineID
		) {
			return "stale";
		}
		setEvents(refreshedEvents);
		const repairEventStatus = latestStatusForResource(
			refreshedEvents,
			resourceID,
		);
		if (repairEventStatus === "in_sync") {
			setSelectedRepairResourceId("");
			setActivityNotice("복구 성공");
			return "terminal";
		}
		if (isBlockedRepairStatus(repairEventStatus)) {
			setSelectedRepairResourceId("");
			setActivityNotice("복구 차단됨");
			return "terminal";
		}
		const refreshedMachine = refreshed?.machines.find(
			(machine) => machine.id === machineID,
		);
		if (resourceID === "" && refreshedMachine?.status === "healthy") {
			setActivityNotice("복구 성공");
			return "terminal";
		}
		if (resourceID === "" && refreshedMachine?.status === "blocked") {
			setActivityNotice("복구 차단됨");
			return "terminal";
		}
		return "pending";
	}

	function scheduleRepairPoll(
		machineID: string,
		resourceID: string,
		repairID: number,
		attempt: number,
	): void {
		if (attempt > maxRepairPolls) {
			return;
		}
		repairPollTimeout.current = window.setTimeout(() => {
			void (async () => {
				const outcome = await refreshRepairOutcome(
					machineID,
					resourceID,
					repairID,
				);
				if (outcome === "pending") {
					scheduleRepairPoll(machineID, resourceID, repairID, attempt + 1);
				}
			})();
		}, repairPollDelayMs);
	}

	return {
		activityNotice,
		clearRepairPoll,
		events,
		handleOpenLogs,
		handleRepairDrift,
		resetRepairState,
		selectedRepairResourceId,
		setSelectedRepairResourceId,
	};
}

function firstDriftedResourceId(events: readonly MachineEvent[]): string {
	const event = events.find(
		(candidate) =>
			candidate.status === "drifted" && validResourceId(candidate.resourceId),
	);
	return event?.resourceId ?? "";
}

function latestStatusForResource(
	events: readonly MachineEvent[],
	resourceId: string,
): string {
	if (resourceId === "") {
		return "";
	}
	const event = events.find((candidate) => candidate.resourceId === resourceId);
	return event?.status ?? "";
}

function isBlockedRepairStatus(status: string): boolean {
	return status === "blocked" || status === "unsupported_adapter";
}

function validResourceId(value: string | undefined): value is string {
	return value !== undefined && value !== "";
}
