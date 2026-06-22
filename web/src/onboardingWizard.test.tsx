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

	it("creates an invite and renders the packaged-client login command", async () => {
		const calls = stubFetchSequence([
			{ code: "pair_123", expiresAt: "2026-06-06T12:10:00Z" },
		]);

		await renderWizard();
		const commands = renderedCommands();

		expect(calls[0]).toEqual({ url: "/api/pair/init", method: "POST" });
		expect(document.body.textContent).toContain("명령 실행 대기 중");
		expect(document.body.textContent).toContain(
			"macOS local QA: unsigned dev .pkg",
		);
		expect(document.body.textContent).toContain(
			"Production macOS: Developer ID Application/Installer, notarization, stapling",
		);
		expect(document.body.textContent).toContain(
			"Linux: Debian/Ubuntu .deb 또는 tarball",
		);
		expect(document.body.textContent).toContain(
			"Run with packaged neul client:",
		);
		expect(document.body.textContent).toContain(
			"packaged approval flow가 준비되기 전에는 fallback/debug 명령으로 등록하세요:",
		);
		expect(commands).toEqual([
			"neul login --server http://localhost:3000",
			"go run ./cmd/neul agent enroll --server http://localhost:3000 --pair pair_123 --connect-once",
		]);
		expect(commands[0]).not.toContain("pair_123");
		expect(commands[0]).not.toContain("--pair");
		expect(commands[0]).not.toContain("go run ./cmd/neul");
		expect(commands[1]).toContain("pair_123");
		expect(commands[1]).toContain("--pair");
		expect(commands[1]).toContain("go run ./cmd/neul");
		expect(document.body.textContent).not.toContain("setup_");
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
			},
			{ resources: [] },
		]);

		await renderWizard();
		await advanceTimers(2000);

		expect(document.body.textContent).toContain("agent 연결 확인 중");
	});

	it("connects when primary login creates a heartbeating machine", async () => {
		vi.useFakeTimers();
		const onConnected = vi.fn();
		stubPrimaryLoginHeartbeat();

		await renderWizard({ onConnected });
		await advanceTimers(2000);

		expect(onConnected).toHaveBeenCalledTimes(1);
		expect(document.body.textContent).toContain("머신이 연결되었습니다.");
	});

	it("shows retry copy when the invite expires", async () => {
		vi.useFakeTimers();
		stubExpiredInvite();

		await renderWizard();
		await advanceTimers(2000);

		expect(document.body.textContent).toContain("등록 시간이 만료되었습니다");
		expect(document.body.textContent).toContain("다시 만들기");
	});

	it("keeps neul up guidance when a claimed invite has no browser heartbeat", async () => {
		vi.useFakeTimers();
		stubClaimedInviteWithoutHeartbeat();

		await renderWizard();
		await advanceTimers(2000);
		await advanceTimers(60 * 2 * 1000);

		expect(document.body.textContent).toContain("agent 연결 확인 중");
		expect(document.body.textContent).toContain("neul up");
		expect(document.body.textContent).not.toContain("agent 응답 없음");
	});

	it("notifies the shell when invite creation loses owner session", async () => {
		const ownerSessionRequired = vi.fn();
		vi.stubGlobal(
			"fetch",
			async () =>
				new Response(
					JSON.stringify({
						error: { code: "unauthorized", message: "Owner session required" },
					}),
					{ status: 401, headers: { "Content-Type": "application/json" } },
				),
		);

		await renderWizard({ onOwnerSessionRequired: ownerSessionRequired });

		expect(ownerSessionRequired).toHaveBeenCalledTimes(1);
		expect(document.body.textContent).not.toContain("등록 오류");
	});
});

async function renderWizard({
	onConnected,
	onOwnerSessionRequired,
}: {
	readonly onConnected?: () => void;
	readonly onOwnerSessionRequired?: () => void;
} = {}): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	const ownerSessionProps =
		onOwnerSessionRequired === undefined ? {} : { onOwnerSessionRequired };
	await act(async () => {
		root.render(
			<OnboardingWizard
				onClose={() => {
					return;
				}}
				onConnected={onConnected ?? (() => {})}
				{...ownerSessionProps}
			/>,
		);
	});
	await act(async () => {
		await Promise.resolve();
	});
}

function renderedCommands(): string[] {
	return Array.from(document.querySelectorAll("code")).map(
		(element) => element.textContent ?? "",
	);
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

function stubPrimaryLoginHeartbeat(): void {
	stubReadyPolls({
		pair: { status: "pending", expiresAt: "2026-06-06T12:10:00Z" },
		dashboard: {
			...emptyDashboard(),
			machines: [
				{
					id: "machine_primary",
					name: "primary-login",
					os: "darwin",
					arch: "arm64",
					agentVersion: "0.1.0",
					status: "healthy",
					lastHeartbeatAt: "2026-06-06T12:01:00Z",
					driftCount: 0,
					pendingCount: 0,
					blockedCount: 0,
					resourceCount: 0,
					appliedCount: 0,
				},
			],
		},
	});
}

function stubExpiredInvite(): void {
	stubReadyPolls({
		pair: { status: "expired", expiresAt: "2026-06-06T12:10:00Z" },
		dashboard: emptyDashboard(),
	});
}

function stubReadyPolls({
	pair,
	dashboard,
}: {
	readonly pair: unknown;
	readonly dashboard: unknown;
}): void {
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url === "/api/pair/init" && init?.method === "POST") {
				return jsonResponse({
					code: "pair_123",
					expiresAt: "2026-06-06T12:10:00Z",
				});
			}
			if (url.startsWith("/api/pair/poll")) {
				return jsonResponse(pair);
			}
			if (url === "/api/dashboard") {
				return jsonResponse(dashboard);
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			return new Response("not found", { status: 404 });
		},
	);
}

function stubClaimedInviteWithoutHeartbeat(): void {
	vi.stubGlobal(
		"fetch",
		async (input: RequestInfo | URL, init?: RequestInit) => {
			const url = String(input);
			if (url === "/api/pair/init" && init?.method === "POST") {
				return jsonResponse({
					code: "pair_123",
					expiresAt: "2026-06-06T12:10:00Z",
				});
			}
			if (url.startsWith("/api/pair/poll")) {
				return jsonResponse({
					status: "claimed",
					machineId: "machine_1",
					expiresAt: "2026-06-06T12:10:00Z",
				});
			}
			if (url === "/api/dashboard") {
				return jsonResponse(emptyDashboard());
			}
			if (url === "/api/resources") {
				return jsonResponse({ resources: [] });
			}
			return new Response("not found", { status: 404 });
		},
	);
}

function emptyDashboard(): unknown {
	return {
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
	};
}

function jsonResponse(body: unknown): Response {
	return new Response(JSON.stringify(body), {
		status: 200,
		headers: { "Content-Type": "application/json" },
	});
}
