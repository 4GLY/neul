import { afterEach, describe, expect, it, vi } from "vitest";
import {
	createPairingInvite,
	loadDashboardData,
	pollPairingInvite,
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
							resourceCount: 1,
							appliedCount: 0,
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
		expect(data.machines[0]?.progress).toBe("0 / 1");
		expect(data.resources[0]?.name).toBe("kubectl");
	});

	it("maps status metrics and machine source fields from the API", async () => {
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				return jsonResponse({
					metrics: {
						total: 99,
						healthy: 0,
						drifted: 0,
						pending: 0,
						offline: 1,
						blocked: 1,
						unknown: 0,
					},
					machines: [
						{
							id: "machine_blocked_api",
							name: "blocked-api",
							os: "linux",
							arch: "x86_64",
							agentVersion: "0.1.0",
							status: "blocked",
							lastHeartbeatAt: "2026-06-05T13:00:00Z",
							lastReconcileAt: "2026-06-05T12:58:00Z",
							driftCount: 1,
							pendingCount: 0,
							blockedCount: 2,
							resourceCount: 4,
							appliedCount: 2,
						},
						{
							id: "machine_unknown_api",
							name: "unknown-api",
							os: "darwin",
							arch: "arm64",
							agentVersion: "",
							status: "unknown",
							driftCount: 0,
							pendingCount: 0,
							blockedCount: 0,
							resourceCount: 0,
							appliedCount: 0,
						},
					],
					activity: [],
					ledger: [],
				});
			}
			return jsonResponse({ resources: [] });
		});

		const data = await loadDashboardData();

		expect(data.metrics).toEqual({
			total: 2,
			healthy: 0,
			drifted: 0,
			pending: 0,
			offline: 1,
			blocked: 1,
			unknown: 0,
		});
		expect(data.machines[0]?.driftCount).toBe(1);
		expect(data.machines[0]?.lastReconcile).toBe("2026-06-05 12:58 UTC");
		expect(data.machines[0]?.lastReconcileAt).toBe("2026-06-05T12:58:00Z");
		expect(data.machines[0]?.lastSeen).toBe("2026-06-05 13:00 UTC");
		expect(data.machines[0]?.progress).toBe("2 / 4");
		expect(data.machines[0]?.note).toBe("Action required");
		expect(data.machines[1]?.status).toBe("unknown");
		expect(data.machines[1]?.desiredState).toBe("Unknown");
	});

	it("uses the no-data timestamp sentinel for invalid API timestamps", async () => {
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/dashboard") {
				return jsonResponse({
					metrics: {
						total: 1,
						healthy: 1,
						drifted: 0,
						pending: 0,
						offline: 0,
						blocked: 0,
						unknown: 0,
					},
					machines: [
						{
							id: "machine_bad_timestamp",
							name: "bad-timestamp",
							os: "darwin",
							arch: "arm64",
							agentVersion: "0.1.0",
							status: "healthy",
							lastHeartbeatAt: "not-a-date",
							lastReconcileAt: "not-a-date",
							driftCount: 0,
							pendingCount: 0,
							blockedCount: 0,
							resourceCount: 1,
							appliedCount: 1,
						},
					],
					activity: [],
					ledger: [],
				});
			}
			return jsonResponse({ resources: [] });
		});

		const data = await loadDashboardData();

		expect(data.machines[0]?.lastReconcile).toBe("아직 없음");
		expect(data.machines[0]?.lastSeen).toBe("아직 없음");
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

function jsonResponse(body: unknown, status = 200): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json" },
	});
}
