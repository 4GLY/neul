import { act } from "react";
import { createRoot } from "react-dom/client";
import { vi } from "vitest";
import { App } from "./App";
import type { ApiMachine } from "./apiTypes";
import type { MachineStatus } from "./types";

export type FetchCall = {
	readonly url: string;
	readonly method: string | undefined;
};

export async function renderApp(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<App />);
	});
	await flushApp();
}

export async function renderWithoutFlush(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<App />);
	});
}

export async function flushApp(): Promise<void> {
	await act(async () => {
		await Promise.resolve();
	});
}

export function stubFetchSequence(bodies: readonly unknown[]): FetchCall[] {
	let index = 0;
	const calls: FetchCall[] = [];
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			calls.push({ url: String(input), method: init?.method });
			if (index >= bodies.length) {
				throw new Error(`unexpected fetch call ${index + 1}: ${String(input)}`);
			}
			const body = bodies[index];
			if (body === undefined) {
				throw new Error(`missing response body ${index + 1}: ${String(input)}`);
			}
			index += 1;
			return jsonResponse(body);
		},
	);
	return calls;
}

export function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

export function getButton(name: string): HTMLButtonElement {
	const button = Array.from(document.querySelectorAll("button")).find(
		(candidate) => candidate.textContent === name,
	);
	if (!(button instanceof HTMLButtonElement)) {
		throw new Error(`button not found: ${name}`);
	}
	return button;
}

export function getButtonContaining(name: string): HTMLButtonElement {
	const button = Array.from(document.querySelectorAll("button")).find(
		(candidate) => candidate.textContent?.includes(name) ?? false,
	);
	if (!(button instanceof HTMLButtonElement)) {
		throw new Error(`button not found: ${name}`);
	}
	return button;
}

type ApiMachineFixture = ApiMachine;

type ApiMachineOptions = {
	readonly lastHeartbeatAt?: string;
	readonly lastReconcileAt?: string;
	readonly resourceCount?: number;
	readonly appliedCount?: number;
};

export function dashboardResponse(
	machines: readonly ApiMachineFixture[],
): unknown {
	return {
		metrics: {
			total: machines.length,
			healthy: countMachines(machines, "healthy"),
			drifted: countMachines(machines, "drifted"),
			pending: countMachines(machines, "pending"),
			offline: countMachines(machines, "offline"),
			blocked: countMachines(machines, "blocked"),
			unknown: countMachines(machines, "unknown"),
		},
		machines,
		activity: [],
		ledger: [],
	};
}

function countMachines(
	machines: readonly ApiMachineFixture[],
	status: MachineStatus,
): number {
	return machines.filter((machine) => machine.status === status).length;
}

export function apiMachine(
	id: string,
	name: string,
	status: MachineStatus,
	options: ApiMachineOptions = {},
): ApiMachineFixture {
	const defaultHeartbeat =
		status === "offline" ? "2026-06-05T12:52:00Z" : "2026-06-05T13:00:00Z";
	const hasDefaultReport = status !== "unknown";
	return {
		id,
		name,
		os: "darwin",
		arch: "arm64",
		agentVersion: "0.1.0",
		status,
		...(options.lastHeartbeatAt === undefined && !hasDefaultReport
			? {}
			: { lastHeartbeatAt: options.lastHeartbeatAt ?? defaultHeartbeat }),
		...(options.lastReconcileAt === undefined && !hasDefaultReport
			? {}
			: { lastReconcileAt: options.lastReconcileAt ?? "2026-06-05T12:58:00Z" }),
		driftCount: status === "drifted" ? 1 : 0,
		pendingCount: status === "pending" ? 1 : 0,
		blockedCount: status === "blocked" ? 1 : 0,
		resourceCount: options.resourceCount ?? (status === "unknown" ? 0 : 1),
		appliedCount: options.appliedCount ?? (status === "healthy" ? 1 : 0),
	};
}
