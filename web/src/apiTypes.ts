import type { MachineStatus } from "./types";

export type ApiMachine = {
	readonly id: string;
	readonly name: string;
	readonly os: string;
	readonly arch: string;
	readonly agentVersion: string;
	readonly status: MachineStatus;
	readonly lastHeartbeatAt?: string;
	readonly lastReconcileAt?: string;
	readonly driftCount: number;
	readonly pendingCount: number;
	readonly blockedCount: number;
	readonly resourceCount: number;
	readonly appliedCount: number;
};

export type ApiDashboardMetrics = {
	readonly total: number;
	readonly healthy: number;
	readonly drifted: number;
	readonly pending: number;
	readonly offline: number;
	readonly blocked: number;
	readonly unknown: number;
};

export type ApiDashboard = {
	readonly metrics: ApiDashboardMetrics;
	readonly machines: readonly ApiMachine[];
	readonly activity: readonly unknown[];
	readonly ledger: readonly unknown[];
	readonly emptyState?: {
		readonly action: string;
		readonly title?: string;
	};
};

export type ApiResource = {
	readonly id: string;
	readonly kind: "package" | "dotfile";
	readonly name: string;
	readonly desiredVersion: number;
	readonly agentSupport: "supported" | "unsupported";
	readonly spec: Readonly<Record<string, unknown>> & {
		readonly desiredVersion?: unknown;
		readonly sourceKind?: unknown;
	};
};

export type ApiResources = {
	readonly resources: readonly ApiResource[];
};

export type ApiMachineEvent = {
	readonly id: string;
	readonly resourceId?: string;
	readonly status: string;
	readonly message: string;
	readonly createdAt: string;
};

export type ApiMachineDetail = {
	readonly events: readonly ApiMachineEvent[];
};

export type ApiPairInitResponse = {
	readonly code: string;
	readonly expiresAt: string;
};

export type ApiPairPollResponse =
	| {
			readonly status: "pending";
			readonly expiresAt: string;
	  }
	| {
			readonly status: "claimed";
			readonly machineId: string;
			readonly expiresAt: string;
	  }
	| {
			readonly status: "expired";
			readonly expiresAt: string;
	  };
