import { act } from "react";
import { createRoot } from "react-dom/client";
import { vi } from "vitest";
import { App } from "./App";

export type RepairCall = {
	readonly url: string;
	readonly method: string | undefined;
	readonly body: BodyInit | null | undefined;
};

export function stubDashboardFetch(calls: string[]): void {
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const path = String(input);
			calls.push(`${init?.method ?? "GET"} ${path}`);
			if (path === "/api/dashboard") {
				return jsonResponse(driftDashboard(1));
			}
			if (path === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			if (path === "/api/machines/machine_1") {
				return jsonResponse({
					events: [
						{
							id: "event_1",
							status: "drifted",
							message: "kubectl missing",
							createdAt: "2026-06-05T13:00:00Z",
						},
					],
				});
			}
			return jsonResponse({ commandId: "command_1", status: "queued" }, 202);
		},
	);
}

export function stubSelectableRepairFetch(
	outcome: "healthy" | "blocked",
): readonly RepairCall[] {
	const calls: RepairCall[] = [];
	let dashboardCalls = 0;
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const path = String(input);
			calls.push({ url: path, method: init?.method, body: init?.body });
			if (path === "/api/dashboard") {
				dashboardCalls += 1;
				return jsonResponse(
					driftDashboard(dashboardCalls === 1 ? 1 : 0, {
						appliedCount: outcome === "healthy" && dashboardCalls > 1 ? 1 : 0,
					}),
				);
			}
			if (path === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			if (path === "/api/machines/machine_1") {
				const status =
					dashboardCalls > 1
						? outcome === "healthy"
							? "in_sync"
							: "blocked"
						: "drifted";
				return jsonResponse({ events: [resourceEvent(status)] });
			}
			return jsonResponse({ commandId: "command_1", status: "queued" }, 202);
		},
	);
	return calls;
}

export function stubPolledRepairFetch(): void {
	let machineDetailCalls = 0;
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const path = String(input);
			if (path === "/api/dashboard") {
				return jsonResponse(
					driftDashboard(1, {
						appliedCount: machineDetailCalls > 1 ? 1 : 0,
					}),
				);
			}
			if (path === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			if (path === "/api/machines/machine_1") {
				machineDetailCalls += 1;
				return jsonResponse({
					events: [
						resourceEvent(machineDetailCalls > 2 ? "in_sync" : "drifted"),
					],
				});
			}
			if (init?.method === "POST" && path.includes("repair-drift")) {
				return jsonResponse({ commandId: "command_1", status: "queued" }, 202);
			}
			throw new Error(`unexpected fetch: ${path}`);
		},
	);
}

export async function renderApp(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<App />);
	});
	await flushApp();
}

export async function click(name: string): Promise<void> {
	const button = [...document.querySelectorAll("button")].find(
		(item) => item.textContent === name,
	);
	if (button === undefined) {
		throw new Error(`button ${name} not found`);
	}
	await act(async () => {
		button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	});
	await flushApp();
}

export async function flushApp(): Promise<void> {
	await act(async () => {
		await Promise.resolve();
	});
}

function driftDashboard(
	driftCount: number,
	options: { readonly appliedCount?: number } = {},
): unknown {
	return {
		metrics: {
			total: 1,
			healthy: 0,
			drifted: 1,
			pending: 0,
			offline: 0,
			blocked: 0,
			unknown: 0,
		},
		machines: [
			{
				id: "machine_1",
				name: "work-macbook",
				os: "darwin",
				arch: "arm64",
				agentVersion: "0.1.0",
				status: "drifted",
				lastHeartbeatAt: "2026-06-05T13:00:00Z",
				lastReconcileAt: "2026-06-05T13:01:00Z",
				driftCount,
				pendingCount: 0,
				blockedCount: 0,
				resourceCount: 1,
				appliedCount: options.appliedCount ?? 0,
			},
		],
		activity: [],
		ledger: [],
	};
}

function resourceEvent(status: string): unknown {
	return {
		id: `event_${status}`,
		resourceId: "resource_brew",
		status,
		message: "kubectl missing",
		createdAt: "2026-06-05T13:00:00Z",
	};
}

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}
