import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	apiMachine,
	dashboardResponse,
	flushApp,
	getButton,
	getButtonContaining,
	jsonResponse,
	renderApp,
	renderWithoutFlush,
} from "./appTestHarness";

describe("App recovery and dashboard state boundaries", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("does not render zero metrics while the initial dashboard load is pending", async () => {
		vi.stubGlobal(
			"fetch",
			async () =>
				new Promise<Response>(() => {
					return;
				}),
		);

		await renderWithoutFlush();

		expect(document.querySelector(".metric-strip")).toBeNull();
		expect(document.body.textContent).toContain("불러오는 중");
	});

	it("recovers from an error state when a later dashboard refresh succeeds", async () => {
		let dashboardCalls = 0;
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				dashboardCalls += 1;
				if (dashboardCalls === 1) {
					return jsonResponse({ error: { code: "server_error" } }, 500);
				}
				return jsonResponse(
					dashboardResponse([
						apiMachine("machine_recovered", "recovered-api", "healthy"),
					]),
				);
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			throw new Error(`unexpected fetch: ${url}`);
		});

		await renderApp();
		expect(document.body.textContent).toContain(
			"대시보드를 불러오지 못했습니다",
		);
		expect(document.querySelector(".metric-strip")).toBeNull();

		await act(async () => {
			getButton("다시 시도").click();
		});
		await flushApp();

		expect(document.body.textContent).toContain("recovered-api");
		expect(document.body.textContent).not.toContain(
			"대시보드를 불러오지 못했습니다",
		);
	});

	it("keeps the last ready dashboard visible when a refresh fails", async () => {
		let dashboardCalls = 0;
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				dashboardCalls += 1;
				if (dashboardCalls > 1) {
					return jsonResponse({ error: { code: "server_error" } }, 500);
				}
				return jsonResponse(
					dashboardResponse([
						apiMachine("machine_stale", "stale-but-visible", "drifted"),
					]),
				);
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			if (url.includes("repair-drift")) {
				return jsonResponse({}, 202);
			}
			throw new Error(`unexpected fetch: ${url}`);
		});

		await renderApp();

		await act(async () => {
			getButton("drift 복구").click();
			await Promise.resolve();
		});
		await flushApp();

		expect(document.body.textContent).toContain(
			"대시보드를 불러오지 못했습니다",
		);
		expect(document.body.textContent).toContain("stale-but-visible");
		expect(document.querySelector(".metric-strip")).not.toBeNull();
	});

	it("renders a connected unknown machine without treating it as just seen", async () => {
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				return jsonResponse(
					dashboardResponse([
						apiMachine("machine_healthy", "healthy-connected", "healthy"),
						apiMachine("machine_unknown", "unknown-connected", "unknown", {
							lastHeartbeatAt: "2026-06-05T12:59:00Z",
							lastReconcileAt: "2026-06-05T12:58:00Z",
							resourceCount: 2,
							appliedCount: 1,
						}),
					]),
				);
			}
			return jsonResponse({ resources: [] });
		});

		await renderApp();

		expect(document.body.textContent).toContain("unknown-connected");
		expect(document.body.textContent).toContain("1 online");
		expect(document.body.textContent).toContain("2026-06-05 12:59 UTC");
		expect(document.body.textContent).toContain("Awaiting first report");
		expect(document.body.textContent).not.toContain("just now");

		const statusFilter = document.querySelector("select");
		if (statusFilter === null) {
			throw new Error("status filter not found");
		}
		await act(async () => {
			statusFilter.value = "unknown";
			statusFilter.dispatchEvent(new Event("change", { bubbles: true }));
		});

		const rows = Array.from(document.querySelectorAll(".machine-grid.row"));
		expect(rows).toHaveLength(1);
		expect(rows[0]?.textContent).toContain("unknown-connected");
	});

	it("ignores stale log responses after another machine is selected", async () => {
		let releaseEvents: ((response: Response) => void) | undefined;
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				return jsonResponse(
					dashboardResponse([
						apiMachine("machine_first", "first-api", "healthy"),
						apiMachine("machine_selected", "selected-api", "drifted"),
					]),
				);
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			if (url === "/api/machines/machine_selected") {
				return new Promise<Response>((resolve) => {
					releaseEvents = resolve;
				});
			}
			throw new Error(`unexpected fetch: ${url}`);
		});

		await renderApp();

		await act(async () => {
			getButtonContaining("selected-api").click();
		});
		await act(async () => {
			getButton("로그 열기").click();
			await Promise.resolve();
		});
		await act(async () => {
			getButtonContaining("first-api").click();
		});
		if (releaseEvents === undefined) {
			throw new Error("events request was not started");
		}
		const resolveEvents = releaseEvents;
		await act(async () => {
			resolveEvents(
				jsonResponse({
					events: [
						{
							id: "event_selected",
							status: "drifted",
							message: "selected-event",
							createdAt: "2026-06-05T13:00:00Z",
						},
					],
				}),
			);
			await Promise.resolve();
		});

		expect(document.body.textContent).not.toContain("selected-event");
	});
});
