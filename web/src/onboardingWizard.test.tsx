import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OnboardingWizard } from "./OnboardingWizard";

describe("OnboardingWizard", () => {
	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("creates an invite and renders the checkout-scoped enroll command", async () => {
		const calls = stubFetchSequence([
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
		]);

		await renderWizard();

		expect(calls[0]).toEqual({ url: "/api/pair/init", method: "POST" });
		expect(document.body.textContent).toContain("명령 실행 대기 중");
		expect(document.body.textContent).toContain("Run from your neul checkout:");
		expect(document.body.textContent).toContain(
			"go run ./cmd/neul agent enroll --server http://localhost:3000 --pair pair_123 --connect-once",
		);
	});

	it("moves to heartbeat waiting when the pair poll is claimed", async () => {
		vi.useFakeTimers();
		stubFetchSequence([
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
			{
				status: "claimed",
				machineId: "machine_1",
				expiresAt: "2026-06-06T12:10:00Z",
			},
			{
				metrics: { total: 0, healthy: 0, drifted: 0, pending: 0 },
				machines: [],
				activity: [],
				ledger: [],
			},
			{ resources: [] },
		]);

		await renderWizard();
		await advanceTimers(2000);

		expect(document.body.textContent).toContain("agent 연결 확인 중");
	});

	it("shows retry copy when the invite expires", async () => {
		vi.useFakeTimers();
		stubFetchSequence([
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
			{ status: "expired", expiresAt: "2026-06-06T12:10:00Z" },
		]);

		await renderWizard();
		await advanceTimers(2000);

		expect(document.body.textContent).toContain("등록 시간이 만료되었습니다");
		expect(document.body.textContent).toContain("다시 만들기");
	});

	it("stops waiting after the claimed machine misses the heartbeat window", async () => {
		vi.useFakeTimers();
		stubFetchSequence([
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
			{
				status: "claimed",
				machineId: "machine_1",
				expiresAt: "2026-06-06T12:10:00Z",
			},
			{
				metrics: { total: 0, healthy: 0, drifted: 0, pending: 0 },
				machines: [],
				activity: [],
				ledger: [],
			},
			{ resources: [] },
		]);

		await renderWizard();
		await advanceTimers(2000);
		await advanceTimers(120_000);

		expect(document.body.textContent).toContain("agent 응답 없음");
	});
});

async function renderWizard(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(
			<OnboardingWizard
				onClose={() => {
					return;
				}}
				onConnected={() => {
					return;
				}}
			/>,
		);
	});
	await act(async () => {
		await Promise.resolve();
	});
}

async function advanceTimers(milliseconds: number): Promise<void> {
	await act(async () => {
		await vi.advanceTimersByTimeAsync(milliseconds);
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
