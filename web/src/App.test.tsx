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
