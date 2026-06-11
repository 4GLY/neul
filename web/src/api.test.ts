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

describe("resource mutation API", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("sends dotfile updates as JSON PATCH requests", async () => {
		const api = await loadUpdateResourceApi();
		const calls: {
			readonly url: string;
			readonly method: string | undefined;
			readonly contentType: string | null;
			readonly body: BodyInit | null | undefined;
		}[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push({
					url: String(input),
					method: init?.method,
					contentType: new Headers(init?.headers).get("Content-Type"),
					body: init?.body,
				});
				return jsonResponse({
					id: "resource_dot_zshrc",
					kind: "dotfile",
					name: "~/.zshrc",
					desiredVersion: 2,
					agentSupport: "supported",
					spec: {
						path: "~/.zshrc",
						content: "export EDITOR=nvim\n",
						mode: "0600",
						applyMode: "symlink",
						targetSegment: "base",
					},
				});
			},
		);

		await api.updateResource("resource_dot_zshrc", {
			path: "~/.zshrc",
			content: "export EDITOR=nvim\n",
			mode: "0600",
			applyMode: "symlink",
			targetSegment: "base",
		});

		expect(calls).toEqual([
			{
				url: "/api/resources/resource_dot_zshrc",
				method: "PATCH",
				contentType: "application/json",
				body: JSON.stringify({
					path: "~/.zshrc",
					content: "export EDITOR=nvim\n",
					mode: "0600",
					applyMode: "symlink",
					targetSegment: "base",
				}),
			},
		]);
	});

	it("maps path_not_allowed update failures to a Korean path error", async () => {
		const api = await loadUpdateResourceApi();
		vi.stubGlobal("fetch", async () =>
			jsonResponse(
				{
					error: {
						code: "path_not_allowed",
						message: "Dotfile path is not allowed.",
					},
				},
				400,
			),
		);

		await expect(
			api.updateResource("resource_dot_zshrc", {
				path: "/etc/hosts",
				content: "x",
				mode: "0644",
				applyMode: "copy",
				targetSegment: "base",
			}),
		).rejects.toThrow("경로를 사용할 수 없습니다");
	});

	it("maps dotfile_invalid update failures to a Korean validation error", async () => {
		const api = await loadUpdateResourceApi();
		vi.stubGlobal("fetch", async () =>
			jsonResponse(
				{
					error: {
						code: "dotfile_invalid",
						message: "Dotfile patch is invalid.",
					},
				},
				400,
			),
		);

		await expect(
			api.updateResource("resource_dot_zshrc", {
				path: "~/.zshrc",
				content: "x",
				mode: "9999",
				applyMode: "copy",
				targetSegment: "base",
			}),
		).rejects.toThrow("모드 또는 적용 방식이 올바르지 않습니다");
	});

	it("sends resource deletes to the resource endpoint", async () => {
		const api = await loadDeleteResourceApi();
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
				return new Response(null, { status: 204 });
			},
		);

		await api.deleteResource("resource_dot_zshrc");

		expect(calls).toEqual([
			{
				url: "/api/resources/resource_dot_zshrc",
				method: "DELETE",
				body: undefined,
			},
		]);
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

type DotfileUpdateInput = {
	readonly path: string;
	readonly content: string;
	readonly mode: string;
	readonly applyMode: "copy" | "symlink";
	readonly targetSegment: string;
};

type UpdateResourceApi = {
	readonly updateResource: (
		id: string,
		input: DotfileUpdateInput,
	) => Promise<unknown>;
};

type DeleteResourceApi = {
	readonly deleteResource: (id: string) => Promise<void>;
};

async function loadUpdateResourceApi(): Promise<UpdateResourceApi> {
	const module = await import("./api");
	if (!hasUpdateResource(module)) {
		throw new Error("updateResource export missing");
	}
	return module;
}

async function loadDeleteResourceApi(): Promise<DeleteResourceApi> {
	const module = await import("./api");
	if (!hasDeleteResource(module)) {
		throw new Error("deleteResource export missing");
	}
	return module;
}

function hasUpdateResource(
	module: typeof import("./api"),
): module is typeof import("./api") & UpdateResourceApi {
	return (
		"updateResource" in module && typeof module.updateResource === "function"
	);
}

function hasDeleteResource(
	module: typeof import("./api"),
): module is typeof import("./api") & DeleteResourceApi {
	return (
		"deleteResource" in module && typeof module.deleteResource === "function"
	);
}
