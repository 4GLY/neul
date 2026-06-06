import type { LucideIcon } from "lucide-react";

export type MachineStatus =
	| "healthy"
	| "drifted"
	| "pending"
	| "offline"
	| "blocked";
export type OsKind = "macOS" | "Linux";
export type SyncState =
	| "applied"
	| "pending"
	| "drifted"
	| "blocked"
	| "rotating"
	| "na";

export type Machine = {
	readonly id: string;
	readonly name: string;
	readonly os: OsKind;
	readonly version: string;
	readonly arch: string;
	readonly tag: string;
	readonly agent: string;
	readonly status: MachineStatus;
	readonly desiredState: string;
	readonly driftCount: number;
	readonly lastReconcile: string;
	readonly lastSeen: string;
	readonly progress: string;
	readonly note: string;
};

export type Activity = {
	readonly id: string;
	readonly tone: "success" | "warning" | "danger" | "info";
	readonly title: string;
	readonly time: string;
	readonly details: readonly string[];
	readonly scope: string;
};

export type ResourceRow = {
	readonly group: "패키지" | "dotfile" | "secret";
	readonly name: string;
	readonly desired: string;
	readonly states: Readonly<Record<string, SyncState>>;
};

export type NavItem = {
	readonly label: string;
	readonly icon: LucideIcon;
};
