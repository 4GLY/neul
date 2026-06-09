import type { Machine } from "./types";

export function selectDashboardMachineId(
	currentMachineId: string,
	machines: readonly Machine[],
): string {
	if (
		currentMachineId !== "" &&
		machines.some((machine) => machine.id === currentMachineId)
	) {
		return currentMachineId;
	}
	return machines[0]?.id ?? "";
}

export function latestReconcileLabel(machines: readonly Machine[]): string {
	let latestMachine: Machine | undefined;
	let latestTime = Number.NEGATIVE_INFINITY;
	for (const machine of machines) {
		if (machine.lastReconcileAt === undefined) {
			continue;
		}
		const time = Date.parse(machine.lastReconcileAt);
		if (
			Number.isNaN(time) ||
			isEarlierReconcile(machine, time, latestMachine, latestTime)
		) {
			continue;
		}
		latestTime = time;
		latestMachine = machine;
	}
	if (latestMachine === undefined) {
		return "아직 없음";
	}
	return `${latestMachine.name} · ${latestMachine.lastReconcile}`;
}

function isEarlierReconcile(
	machine: Machine,
	time: number,
	latestMachine: Machine | undefined,
	latestTime: number,
): boolean {
	if (time < latestTime) {
		return true;
	}
	return (
		time === latestTime &&
		latestMachine !== undefined &&
		machine.name >= latestMachine.name
	);
}
