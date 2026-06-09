import { afterEach, describe, expect, it, vi } from "vitest";
import {
	createLocalSession,
	createPackageResource,
	createPairingInvite,
	LocalSessionError,
	loadDashboardData,
	OwnerSessionRequiredError,
	pollPairingInvite,
	repairDrift,
} from "./api";

describe("loadDashboardData", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("loads dashboard and resources from relative API paths", async () => {
		const calls: string[] = [];
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			calls.push(url);
			if (url === "/api/dashboard") {
				return jsonResponse({
					metrics: { total: 1, healthy: 0, drifted: 1, pending: 0 },
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
			return jsonResponse({
				resources: [
					{
						id: "resource_1",
						kind: "package",
						name: "kubectl",
						desiredVersion: 1,
						agentSupport: "supported",
						spec: { desiredVersion: "latest" },
					},
				],
			});
		});

		const data = await loadDashboardData();

		expect(calls).toEqual(["/api/dashboard", "/api/resources"]);
		expect(data.machines[0]?.name).toBe("work-macbook");
		expect(data.machines[0]?.os).toBe("macOS");
		expect(data.resources[0]?.name).toBe("kubectl");
	});

	it("throws a Korean-friendly error when the API fails", async () => {
		vi.stubGlobal("fetch", async () =>
			jsonResponse({ error: { code: "server_error" } }, 500),
		);

		await expect(loadDashboardData()).rejects.toThrow("대시보드");
	});
});

describe("agent pairing API", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("creates a pairing invite through the relative init endpoint", async () => {
		const calls: {
			readonly url: string;
			readonly method: string | undefined;
			readonly body: BodyInit | null | undefined;
		}[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push({
					url: String(input),
					method: init?.method,
					body: init?.body,
				});
				return jsonResponse({
					code: "pair_123",
					expiresAt: "2026-06-06T12:00:00Z",
				});
			},
		);

		const invite = await createPairingInvite();

		expect(calls).toEqual([
			{ url: "/api/pair/init", method: "POST", body: undefined },
		]);
		expect(invite).toEqual({
			code: "pair_123",
			expiresAt: "2026-06-06T12:00:00Z",
		});
	});

	it("polls a pairing invite with an encoded code parameter", async () => {
		const calls: string[] = [];
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			calls.push(String(input));
			return jsonResponse({
				status: "claimed",
				machineId: "machine_1",
				expiresAt: "2026-06-06T12:00:00Z",
			});
		});

		const result = await pollPairingInvite("pair?code&next");

		expect(calls).toEqual(["/api/pair/poll?code=pair%3Fcode%26next"]);
		expect(result).toEqual({
			status: "claimed",
			machineId: "machine_1",
			expiresAt: "2026-06-06T12:00:00Z",
		});
	});
});

describe("local owner session API", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("exchanges the setup token through a JSON POST body only", async () => {
		const calls: {
			readonly url: string;
			readonly method: string | undefined;
			readonly body: BodyInit | null | undefined;
			readonly credentials: RequestCredentials | undefined;
		}[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push({
					url: String(input),
					method: init?.method,
					body: init?.body,
					credentials: init?.credentials,
				});
				return new Response(null, { status: 204 });
			},
		);

		await createLocalSession("setup_secret");

		expect(calls).toEqual([
			{
				url: "/api/session/local",
				method: "POST",
				body: JSON.stringify({ setupToken: "setup_secret" }),
				credentials: "same-origin",
			},
		]);
		expect(calls[0]?.url).not.toContain("setup_secret");
	});

	it.each([
		["setup_token_invalid", 401],
		["setup_token_used", 409],
		["setup_token_expired", 410],
	] as const)("preserves local-session error code %s", async (code, status) => {
		vi.stubGlobal(
			"fetch",
			async () =>
				new Response(
					JSON.stringify({
						error: { code, message: "setup token failed" },
					}),
					{ status, headers: { "Content-Type": "application/json" } },
				),
		);

		await expect(createLocalSession("setup_bad")).rejects.toBeInstanceOf(
			LocalSessionError,
		);
		await expect(createLocalSession("setup_bad")).rejects.toMatchObject({
			code,
		});
	});
});

describe("owner mutation API auth", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("routes a missing owner session during resource mutation to setup", async () => {
		stubUnauthorized();

		await expect(
			createPackageResource({
				name: "kubectl",
				sourceKind: "brew",
				desiredVersion: "latest",
				targetSegment: "base",
			}),
		).rejects.toBeInstanceOf(OwnerSessionRequiredError);
	});

	it("routes a missing owner session during pairing invite creation to setup", async () => {
		stubUnauthorized();

		await expect(createPairingInvite()).rejects.toBeInstanceOf(
			OwnerSessionRequiredError,
		);
	});

	it("routes a missing owner session during repair drift to setup", async () => {
		stubUnauthorized();

		await expect(repairDrift("machine_1")).rejects.toBeInstanceOf(
			OwnerSessionRequiredError,
		);
	});
});

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}

function stubUnauthorized(): void {
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
}
