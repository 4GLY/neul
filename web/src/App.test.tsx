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
	stubFetchSequence,
} from "./appTestHarness";

describe("App API states", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("renders the first-machine CTA when the API has no machines", async () => {
		const fetchCalls = stubFetchSequence([
			{
				metrics: {
					total: 0,
					healthy: 0,
					drifted: 0,
					pending: 0,
					offline: 0,
					blocked: 0,
					unknown: 0,
				},
				machines: [],
				activity: [],
				ledger: [],
				emptyState: { action: "create_pairing_code" },
			},
			{ resources: [] },
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
		]);

		await renderApp();

		expect(document.body.textContent).toContain("첫 머신을 등록하세요");
		expect(document.body.textContent).toContain("첫 머신 등록");
		expect(document.querySelector(".metric-strip")).toBeNull();

		const button = getButton("첫 머신 등록");
		await act(async () => {
			button.click();
		});

		expect(fetchCalls.at(-1)).toEqual({
			url: "/api/pair/init",
			method: "POST",
		});
		expect(document.body.textContent).toContain("Run from your neul checkout:");
	});

	it("renders a loading state while dashboard data is pending", async () => {
		vi.stubGlobal(
			"fetch",
			async () =>
				new Promise<Response>(() => {
					return;
				}),
		);

		await renderWithoutFlush();

		expect(document.body.textContent).toContain("불러오는 중");
	});

	it("renders a Korean error state when the dashboard API fails", async () => {
		vi.stubGlobal(
			"fetch",
			async () =>
				new Response(`{"error":{"code":"server_error","message":"test"}}`, {
					status: 500,
				}),
		);

		await renderApp();

		expect(document.body.textContent).toContain(
			"대시보드를 불러오지 못했습니다",
		);
	});

	it("returns to setup when an owner action loses its session", async () => {
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				return jsonResponse(dashboardResponse([]));
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			return jsonResponse({ error: { code: "unauthorized" } }, 401);
		});

		await renderApp();
		await clickButton("첫 머신 등록");

		expect(document.body.textContent).toContain("첫 실행 설정");
		expect(document.body.textContent).not.toContain("등록 오류");
	});

	it("renders first-run setup instead of the dashboard error when the owner session is missing", async () => {
		vi.stubGlobal("fetch", async () =>
			jsonResponse({ error: { code: "unauthorized" } }, 401),
		);

		await renderApp();

		expect(document.body.textContent).toContain("첫 실행 설정");
		expect(document.body.textContent).toContain("setup token");
		expect(document.body.textContent).not.toContain(
			"대시보드를 불러오지 못했습니다",
		);
	});

	it("exchanges a setup token and lands on the dashboard", async () => {
		const calls: {
			readonly url: string;
			readonly method: string | undefined;
			readonly body: BodyInit | null | undefined;
		}[] = [];
		let dashboardAttempts = 0;
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				const url = String(input);
				calls.push({ url, method: init?.method, body: init?.body });
				if (url === "/api/session/local") {
					return new Response(null, { status: 204 });
				}
				if (url === "/api/dashboard") {
					dashboardAttempts += 1;
					if (dashboardAttempts === 1) {
						return jsonResponse({ error: { code: "unauthorized" } }, 401);
					}
					return jsonResponse(dashboardResponse([]));
				}
				return jsonResponse({ resources: [] });
			},
		);

		await renderApp();
		await fillSetupToken("setup_secret");
		await clickButton("설정 완료");

		expect(calls).toContainEqual({
			url: "/api/session/local",
			method: "POST",
			body: JSON.stringify({ setupToken: "setup_secret" }),
		});
		expect(window.location.href).not.toContain("setup_secret");
		expect(document.title).not.toContain("setup_secret");
		expect(document.body.textContent).toContain("머신");
		expect(document.body.textContent).toContain("첫 머신 등록");
	});

	it.each([
		["setup_token_invalid", 401, "setup token이 올바르지 않습니다."],
		["setup_token_used", 409, "이미 사용된 setup token입니다."],
		[
			"setup_token_expired",
			410,
			"setup token이 만료되었습니다. 서버 콘솔에 새 setup token을 출력했습니다.",
		],
	] as const)("renders %s setup state", async (code, status, message) => {
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/session/local") {
				return jsonResponse({ error: { code, message } }, status);
			}
			return jsonResponse({ error: { code: "unauthorized" } }, 401);
		});

		await renderApp();
		await fillSetupToken("setup_bad");
		await clickButton("설정 완료");

		expect(document.body.textContent).toContain(message);
		expect(setupTokenInput().value).toBe("");
		expect(document.cookie).not.toContain("neul_session");
		expect(window.location.href).not.toContain("setup_bad");
		expect(document.title).not.toContain("setup_bad");
	});

	it("renders connected dashboard states from API data without mock fallback names", async () => {
		stubFetchSequence([
			dashboardResponse([
				apiMachine("api-healthy", "healthy-api", "healthy"),
				apiMachine("api-pending", "pending-api", "pending"),
				apiMachine("api-drifted", "drifted-api", "drifted"),
				apiMachine("api-blocked", "blocked-api", "blocked"),
				apiMachine("api-unknown", "unknown-api", "unknown"),
				apiMachine("api-offline", "offline-api", "offline"),
			]),
			{ resources: [] },
		]);

		await renderApp();

		expect(document.body.textContent).toContain("healthy-api");
		expect(document.body.textContent).toContain("pending-api");
		expect(document.body.textContent).toContain("drifted-api");
		expect(document.body.textContent).toContain("blocked-api");
		expect(document.body.textContent).toContain("unknown-api");
		expect(document.body.textContent).toContain("offline-api");
		expect(document.body.textContent).toContain("Unknown");
		expect(
			Array.from(document.querySelectorAll(".metric strong")).map(
				(metric) => metric.textContent,
			),
		).toEqual(["6", "1", "1", "1", "blocked-api · 2026-06-05 12:58 UTC"]);
		expect(document.body.textContent).toContain(
			"1 blocked · 1 offline · 1 awaiting report",
		);
		expect(getButtonContaining("offline-api").textContent).toContain(
			"2026-06-05 12:52 UTC",
		);
		expect(document.body.textContent).not.toContain("mac-studio");
		expect(document.body.textContent).not.toContain("work-macbook");
		expect(document.body.textContent).not.toContain("linux-vm");
		expect(document.body.textContent).not.toContain("homelab-node");
	});

	it("falls back selected row and inspector when selected machine disappears after refresh", async () => {
		stubFetchSequence([
			dashboardResponse([
				apiMachine("machine_first", "first-api", "healthy"),
				apiMachine("machine_selected", "selected-api", "drifted"),
			]),
			{ resources: [] },
			{
				events: [
					{
						id: "event_selected",
						status: "drifted",
						message: "selected-event",
						createdAt: "2026-06-05T13:00:00Z",
					},
				],
			},
			{},
			dashboardResponse([apiMachine("machine_first", "first-api", "healthy")]),
			{ resources: [] },
		]);

		await renderApp();

		await act(async () => {
			getButtonContaining("selected-api").click();
		});
		expect(
			document.querySelector(".machine-grid.row.selected")?.textContent,
		).toContain("selected-api");
		expect(document.querySelector(".inspector")?.textContent).toContain(
			"selected-api",
		);
		await act(async () => {
			getButton("로그 열기").click();
		});
		expect(document.body.textContent).toContain("selected-event");

		await act(async () => {
			getButton("drift 복구").click();
		});
		await act(async () => {
			await Promise.resolve();
		});

		expect(
			document.querySelector(".machine-grid.row.selected")?.textContent,
		).toContain("first-api");
		expect(document.querySelector(".inspector")?.textContent).toContain(
			"first-api",
		);
		expect(document.body.textContent).not.toContain("selected-api");
		expect(document.body.textContent).not.toContain("selected-event");
	});

	it("keeps selected row and inspector stable when refresh still returns it", async () => {
		stubFetchSequence([
			dashboardResponse([
				apiMachine("machine_first", "first-api", "healthy"),
				apiMachine("machine_selected", "selected-api", "drifted"),
			]),
			{ resources: [] },
			{},
			dashboardResponse([
				apiMachine("machine_first", "first-api", "healthy"),
				apiMachine("machine_selected", "selected-api", "drifted"),
			]),
			{ resources: [] },
		]);

		await renderApp();

		await act(async () => {
			getButtonContaining("selected-api").click();
		});
		await act(async () => {
			getButton("drift 복구").click();
		});
		await flushApp();

		expect(
			document.querySelector(".machine-grid.row.selected")?.textContent,
		).toContain("selected-api");
		expect(document.querySelector(".inspector")?.textContent).toContain(
			"selected-api",
		);
	});

	it("keeps the selected table row mounted while repair refresh is pending", async () => {
		const initialDashboard = dashboardResponse([
			apiMachine("machine_first", "first-api", "healthy"),
			apiMachine("machine_selected", "selected-api", "drifted"),
		]);
		const refreshedDashboard = dashboardResponse([
			apiMachine("machine_first", "first-api", "healthy"),
			apiMachine("machine_selected", "selected-api", "drifted"),
		]);
		let dashboardCalls = 0;
		let releaseDashboard: (() => void) | undefined;
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				const url = String(input);
				if (url === "/api/dashboard") {
					dashboardCalls += 1;
					if (dashboardCalls === 1) {
						return jsonResponse(initialDashboard);
					}
					return new Promise<Response>((resolve) => {
						releaseDashboard = () => resolve(jsonResponse(refreshedDashboard));
					});
				}
				if (url === "/api/resources") {
					return jsonResponse({ resources: [] });
				}
				if (init?.method === "POST" && url.includes("repair-drift")) {
					return jsonResponse({}, 202);
				}
				throw new Error(`unexpected fetch: ${url}`);
			},
		);

		await renderApp();

		await act(async () => {
			getButtonContaining("selected-api").click();
		});
		await act(async () => {
			getButton("drift 복구").click();
			await Promise.resolve();
			await Promise.resolve();
		});

		if (releaseDashboard === undefined) {
			throw new Error("repair refresh did not request dashboard data");
		}
		const completeRefresh = releaseDashboard;
		expect(
			document.querySelector(".machine-grid.row.selected")?.textContent,
		).toContain("selected-api");
		expect(getButtonContaining("selected-api")).toBeInstanceOf(
			HTMLButtonElement,
		);

		await act(async () => {
			completeRefresh();
			await Promise.resolve();
		});
		await flushApp();

		expect(
			document.querySelector(".machine-grid.row.selected")?.textContent,
		).toContain("selected-api");
	});
});

async function clickButton(name: string): Promise<void> {
	const button = getButton(name);
	await act(async () => {
		button.click();
	});
	await flushApp();
}

async function fillSetupToken(value: string): Promise<void> {
	const input = setupTokenInput();
	await act(async () => {
		const valueSetter = Object.getOwnPropertyDescriptor(
			HTMLInputElement.prototype,
			"value",
		)?.set;
		valueSetter?.call(input, value);
		input.dispatchEvent(new Event("input", { bubbles: true }));
	});
}

function setupTokenInput(): HTMLInputElement {
	const input = Array.from(document.querySelectorAll("input")).find(
		(candidate) => candidate.getAttribute("aria-label") === "setup token",
	);
	if (!(input instanceof HTMLInputElement)) {
		throw new Error("setup token input not found");
	}
	return input;
}
