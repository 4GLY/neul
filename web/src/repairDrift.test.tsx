import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	click,
	flushApp,
	type RepairCall,
	renderApp,
	stubDashboardFetch,
	stubPolledRepairFetch,
	stubSelectableRepairFetch,
} from "./repairDriftTestHarness";

describe("repair drift UX", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
		document.body.innerHTML = "";
	});

	it("posts repair drift for the selected machine", async () => {
		const calls: string[] = [];
		stubDashboardFetch(calls);
		await renderApp();

		await click("drift 복구");

		expect(calls).toContain("POST /api/machines/machine_1/repair-drift");
		expect(document.body.textContent).toContain("복구 명령 대기 중");
	});

	it("repairs a selected drifted resource and shows the success outcome", async () => {
		const calls: readonly RepairCall[] = stubSelectableRepairFetch("healthy");
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(calls).toContainEqual({
			url: "/api/machines/machine_1/repair-drift",
			method: "POST",
			body: JSON.stringify({ resourceIds: ["resource_brew"] }),
		});
		expect(document.body.textContent).toContain("복구 성공");
	});

	it("shows the blocked repair outcome after a repair refresh reports blocked", async () => {
		stubSelectableRepairFetch("blocked");
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(document.body.textContent).toContain("복구 차단됨");
	});

	it("keeps pending visible until a later repair poll reports success", async () => {
		vi.useFakeTimers();
		stubPolledRepairFetch();
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(document.body.textContent).toContain("복구 명령 대기 중");
		expect(document.body.textContent).not.toContain("복구 성공");

		await act(async () => {
			await vi.advanceTimersByTimeAsync(5000);
		});
		await flushApp();

		expect(document.body.textContent).toContain("복구 성공");
	});

	it("does not report command creation failure when post succeeds but refresh fails", async () => {
		let dashboardCalls = 0;
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				const path = String(input);
				if (path === "/api/dashboard") {
					dashboardCalls += 1;
					if (dashboardCalls === 1) {
						return new Response(JSON.stringify(driftDashboardForTest()), {
							headers: { "Content-Type": "application/json" },
						});
					}
					return new Response(JSON.stringify({ error: { code: "server" } }), {
						headers: { "Content-Type": "application/json" },
						status: 500,
					});
				}
				if (path === "/api/resources") {
					return new Response(JSON.stringify({ resources: [] }), {
						headers: { "Content-Type": "application/json" },
					});
				}
				if (init?.method === "POST" && path.includes("repair-drift")) {
					return new Response(JSON.stringify({ commandId: "command_1" }), {
						headers: { "Content-Type": "application/json" },
						status: 202,
					});
				}
				throw new Error(`unexpected fetch: ${path}`);
			},
		);
		await renderApp();

		await click("drift 복구");

		expect(document.body.textContent).not.toContain(
			"복구 명령을 만들지 못했습니다",
		);
		expect(document.body.textContent).toContain(
			"복구 상태를 새로고침하지 못했습니다",
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

function driftDashboardForTest(): unknown {
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
				driftCount: 1,
				pendingCount: 0,
				blockedCount: 0,
				resourceCount: 1,
				appliedCount: 0,
			},
		],
		activity: [],
		ledger: [],
	};
}
