import type {
	ApiDashboard,
	ApiMachineDetail,
	ApiMachineEvent,
	ApiPairInitResponse,
	ApiPairPollResponse,
	ApiResource,
	ApiResources,
} from "./apiTypes";
import type { Activity, Machine, ResourceRow, SyncState } from "./types";

export type DashboardData = {
	readonly machines: readonly Machine[];
	readonly resources: readonly ResourceRow[];
	readonly activities: readonly Activity[];
	readonly emptyStateAction?: string;
};

export type MachineEvent = ApiMachineEvent;

export async function createPairingInvite(): Promise<ApiPairInitResponse> {
	const response = await fetch("/api/pair/init", { method: "POST" });
	if (!response.ok) {
		throw new Error("페어링 초대를 만들지 못했습니다");
	}
	return (await response.json()) as ApiPairInitResponse;
}

export async function pollPairingInvite(
	code: string,
): Promise<ApiPairPollResponse> {
	return fetchJSON<ApiPairPollResponse>(
		`/api/pair/poll?code=${encodeURIComponent(code)}`,
	);
}

export async function loadDashboardData(): Promise<DashboardData> {
	const [dashboard, resources] = await Promise.all([
		fetchJSON<ApiDashboard>("/api/dashboard"),
		fetchJSON<ApiResources>("/api/resources"),
	]);
	const data: DashboardData = {
		machines: dashboard.machines.map(mapMachine),
		resources: resources.resources.map(mapResource),
		activities: mapActivities(dashboard),
	};
	if (dashboard.emptyState?.action !== undefined) {
		return { ...data, emptyStateAction: dashboard.emptyState.action };
	}
	return data;
}

export async function createPackageResource(input: {
	readonly name: string;
	readonly sourceKind: "brew" | "apt" | "mise";
	readonly desiredVersion: string;
	readonly targetSegment: string;
}): Promise<ApiResource> {
	return postResource("/api/resources/package", input);
}

export async function createDotfileResource(input: {
	readonly path: string;
	readonly content: string;
	readonly mode: string;
	readonly applyMode: "copy" | "symlink";
	readonly targetSegment: string;
}): Promise<ApiResource> {
	return postResource("/api/resources/dotfile", input);
}

export async function repairDrift(machineId: string): Promise<void> {
	const response = await fetch(`/api/machines/${machineId}/repair-drift`, {
		method: "POST",
		headers: { "Idempotency-Key": `web-repair-${machineId}-${Date.now()}` },
	});
	if (!response.ok) {
		throw new Error("drift 복구 명령을 만들지 못했습니다");
	}
}

export async function loadMachineEvents(
	machineId: string,
): Promise<readonly MachineEvent[]> {
	const detail = await fetchJSON<ApiMachineDetail>(
		`/api/machines/${machineId}`,
	);
	return detail.events;
}

async function fetchJSON<T>(path: string): Promise<T> {
	const response = await fetch(path);
	if (!response.ok) {
		throw new Error("대시보드를 불러오지 못했습니다");
	}
	return (await response.json()) as T;
}

async function postResource<T extends object>(
	path: string,
	body: T,
): Promise<ApiResource> {
	const response = await fetch(path, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(body),
	});
	if (!response.ok) {
		const error = (await response.json().catch(() => null)) as {
			readonly error?: { readonly code?: string };
		} | null;
		if (error?.error?.code === "path_not_allowed") {
			throw new Error("경로를 사용할 수 없습니다");
		}
		throw new Error("리소스를 저장하지 못했습니다");
	}
	return (await response.json()) as ApiResource;
}

function mapMachine(machine: ApiDashboard["machines"][number]): Machine {
	const driftCount = machine.driftCount + machine.blockedCount;
	const pendingCount = machine.pendingCount;
	return {
		id: machine.id,
		name: machine.name,
		os: machine.os === "darwin" ? "macOS" : "Linux",
		version: machine.os === "darwin" ? "macOS" : machine.os,
		arch: machine.arch,
		tag: "base",
		agent: machine.agentVersion || "unknown",
		status: machine.status,
		desiredState: desiredStateLabel(machine.status),
		driftCount,
		lastReconcile: "최근 report",
		lastSeen: machine.lastHeartbeatAt === undefined ? "unknown" : "just now",
		progress: `${Math.max(0, 1 - pendingCount)} / 1`,
		note: noteFor(machine.status, driftCount, pendingCount),
	};
}

function mapResource(resource: ApiResource): ResourceRow {
	const group = resource.kind === "package" ? "패키지" : "dotfile";
	const desired =
		typeof resource.spec.desiredVersion === "string"
			? resource.spec.desiredVersion
			: `v${resource.desiredVersion}`;
	return {
		group,
		name: resource.name,
		desired,
		states: {},
	};
}

function mapActivities(_dashboard: ApiDashboard): readonly Activity[] {
	return [];
}

function desiredStateLabel(status: Machine["status"]): string {
	if (status === "healthy") {
		return "In sync";
	}
	if (status === "drifted") {
		return "Drift detected";
	}
	if (status === "pending") {
		return "Applying";
	}
	if (status === "blocked") {
		return "Blocked";
	}
	return "Unknown";
}

function noteFor(
	status: Machine["status"],
	driftCount: number,
	pendingCount: number,
): string {
	if (status === "drifted") {
		return `${driftCount} resources drifted`;
	}
	if (status === "pending") {
		return `${pendingCount} resources pending`;
	}
	if (status === "blocked") {
		return "Action required";
	}
	if (status === "offline") {
		return "Agent reconnecting";
	}
	return "All good";
}

export function syncStateFromStatus(status: Machine["status"]): SyncState {
	if (status === "healthy") {
		return "applied";
	}
	if (status === "blocked") {
		return "blocked";
	}
	if (status === "drifted") {
		return "drifted";
	}
	if (status === "pending") {
		return "pending";
	}
	return "na";
}
