import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

describe("repair drift UX", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("posts repair drift for the selected machine", async () => {
		const calls: string[] = [];
		stubDashboardFetch(calls);
		await renderApp();

		await click("drift 복구");

		expect(calls).toContain("POST /api/machines/machine_1/repair-drift");
		expect(document.body.textContent).toContain(
			"복구 명령을 대기열에 추가했습니다",
		);
	});

	it("opens recent event logs without streaming", async () => {
		const calls: string[] = [];
		stubDashboardFetch(calls);
		await renderApp();

		await click("로그 열기");

		expect(calls).toContain("GET /api/machines/machine_1");
		expect(document.body.textContent).toContain("kubectl missing");
	});
});

function stubDashboardFetch(calls: string[]): void {
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const path = String(input);
			calls.push(`${init?.method ?? "GET"} ${path}`);
			if (path === "/api/dashboard") {
				return jsonResponse({
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
							driftCount: 1,
							pendingCount: 0,
							blockedCount: 0,
						},
					],
					activity: [],
					ledger: [],
				});
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

async function renderApp(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<App />);
	});
	await act(async () => {
		await Promise.resolve();
	});
}

async function click(name: string): Promise<void> {
	const button = [...document.querySelectorAll("button")].find(
		(item) => item.textContent === name,
	);
	if (button === undefined) {
		throw new Error(`button ${name} not found`);
	}
	await act(async () => {
		button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	});
}

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}
