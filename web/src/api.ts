import type {
	ApiDashboard,
	ApiDashboardMetrics,
	ApiMachineDetail,
	ApiMachineEvent,
	ApiPairInitResponse,
	ApiPairPollResponse,
	ApiResource,
	ApiResources,
} from "./apiTypes";
import {
	errorCodeFromResponse,
	OwnerSessionRequiredError,
} from "./localSession";
import type { Activity, Machine, ResourceRow } from "./types";

export {
	createLocalSession,
	LocalSessionError,
	OwnerSessionRequiredError,
} from "./localSession";
export {
	createDotfileResource,
	createPackageResource,
	deleteResource,
	updateResource,
} from "./resourceApi";

export type DashboardData = {
	readonly metrics: DashboardMetrics;
	readonly machines: readonly Machine[];
	readonly resources: readonly ResourceRow[];
	readonly resourceRecords: ApiResources["resources"];
	readonly activities: readonly Activity[];
	readonly emptyStateAction?: string;
};

export type DashboardMetrics = {
	readonly total: number;
	readonly healthy: number;
	readonly drifted: number;
	readonly pending: number;
	readonly offline: number;
	readonly blocked: number;
	readonly unknown: number;
};

export type MachineEvent = ApiMachineEvent;

export async function createPairingInvite(): Promise<ApiPairInitResponse> {
	const response = await fetch("/api/pair/init", { method: "POST" });
	if (!response.ok) {
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
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
	const machines = dashboard.machines.map(mapMachine);
	const data: DashboardData = {
		metrics: mapMetrics(dashboard.metrics, machines.length),
		machines,
		resources: resources.resources.map(mapResource),
		resourceRecords: resources.resources,
		activities: mapActivities(dashboard),
	};
	if (dashboard.emptyState?.action !== undefined) {
		return { ...data, emptyStateAction: dashboard.emptyState.action };
	}
	return data;
}

export async function repairDrift(machineId: string): Promise<void> {
	const response = await fetch(`/api/machines/${machineId}/repair-drift`, {
		method: "POST",
		headers: { "Idempotency-Key": `web-repair-${machineId}-${Date.now()}` },
	});
	if (!response.ok) {
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
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
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
		throw new Error("대시보드를 불러오지 못했습니다");
	}
	return (await response.json()) as T;
}

async function isOwnerSessionRequiredResponse(
	response: Response,
): Promise<boolean> {
	if (response.status !== 401) {
		return false;
	}
	const code = await errorCodeFromResponse(response);
	return code === "unauthorized" || code === "owner_session_required";
}

function mapMetrics(
	metrics: ApiDashboardMetrics,
	machineCount: number,
): DashboardMetrics {
	return {
		total: machineCount,
		healthy: metrics.healthy,
		drifted: metrics.drifted,
		pending: metrics.pending,
		offline: metrics.offline,
		blocked: metrics.blocked,
		unknown: metrics.unknown,
	};
}

function mapMachine(machine: ApiDashboard["machines"][number]): Machine {
	const driftCount = machine.driftCount;
	const pendingCount = machine.pendingCount;
	const resourceCount = machine.resourceCount;
	const appliedCount = machine.appliedCount;
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
		pendingCount,
		blockedCount: machine.blockedCount,
		resourceCount,
		appliedCount,
		lastReconcile: formatTimestamp(machine.lastReconcileAt),
		...(machine.lastReconcileAt === undefined
			? {}
			: { lastReconcileAt: machine.lastReconcileAt }),
		lastSeen: lastSeenLabel(machine.lastHeartbeatAt),
		progress: `${appliedCount} / ${resourceCount}`,
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

function formatTimestamp(value: string | undefined): string {
	if (value === undefined) {
		return "아직 없음";
	}
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return "아직 없음";
	}
	const iso = date.toISOString();
	return `${iso.slice(0, 10)} ${iso.slice(11, 16)} UTC`;
}

function lastSeenLabel(value: string | undefined): string {
	if (value === undefined) {
		return "unknown";
	}
	return formatTimestamp(value);
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
	if (status === "unknown") {
		return "Awaiting first report";
	}
	return "All good";
}
