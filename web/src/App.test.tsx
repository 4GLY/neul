import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

describe("App API states", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("renders the first-machine CTA when the API has no machines", async () => {
		const fetchCalls = stubFetchSequence([
			{
				metrics: { total: 0, healthy: 0, drifted: 0, pending: 0 },
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
				return jsonResponse(emptyDashboard());
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
					return jsonResponse(emptyDashboard());
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
});

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

function stubFetchSequence(bodies: readonly unknown[]): {
	readonly url: string;
	readonly method: string | undefined;
}[] {
	let index = 0;
	const calls: {
		readonly url: string;
		readonly method: string | undefined;
	}[] = [];
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			calls.push({ url: String(input), method: init?.method });
			const body = bodies[index] ?? bodies[bodies.length - 1];
			index += 1;
			return new Response(JSON.stringify(body), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			});
		},
	);
	return calls;
}

function getButton(name: string): HTMLButtonElement {
	const button = Array.from(document.querySelectorAll("button")).find(
		(candidate) => candidate.textContent === name,
	);
	if (!(button instanceof HTMLButtonElement)) {
		throw new Error(`button not found: ${name}`);
	}
	return button;
}

async function clickButton(name: string): Promise<void> {
	const button = getButton(name);
	await act(async () => {
		button.click();
	});
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

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

function emptyDashboard(): unknown {
	return {
		metrics: { total: 0, healthy: 0, drifted: 0, pending: 0 },
		machines: [],
		activity: [],
		ledger: [],
		emptyState: { action: "create_pairing_code" },
	};
}
