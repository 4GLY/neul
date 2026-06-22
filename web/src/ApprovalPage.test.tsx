import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { getButton, jsonResponse, renderApp } from "./appTestHarness";

describe("ApprovalPage", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
		window.history.pushState(null, "", "/");
		document.body.innerHTML = "";
	});

	it("renders owner-session-required copy when status returns 401", async () => {
		openApprovalRoute();
		vi.stubGlobal("fetch", async () =>
			jsonResponse({ error: { code: "owner_session_required" } }, 401),
		);

		await renderApp();

		expect(document.body.textContent).toContain("owner session이 필요합니다");
		expect(document.body.textContent).toContain("이미 로그인된 owner 브라우저");
		expect(document.body.textContent).not.toContain("pair_");
		expect(window.location.href).toContain("approval=approval_123");
		expect(window.location.href).toContain("nonce=nonce_123");
	});

	it("renders machine preview and posts approve decisions without credentials", async () => {
		openApprovalRoute();
		const calls: {
			readonly url: string;
			readonly method: string | undefined;
			readonly body: BodyInit | null | undefined;
		}[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				const url = String(input);
				calls.push({ url, method: init?.method, body: init?.body });
				if (url.startsWith("/api/pair/approval/status")) {
					return jsonResponse({
						status: "pending",
						approvalId: "approval_123",
						expiresAt: "2026-06-19T08:10:00Z",
						csrfToken: "csrf_123",
						comparisonCode: "742-918",
						machine: {
							name: "joon-macbook",
							os: "darwin",
							arch: "arm64",
							agentVersion: "0.1.0",
						},
					});
				}
				return jsonResponse({
					status: "approved",
					expiresAt: "2026-06-19T08:10:00Z",
				});
			},
		);

		await renderApp();
		await clickButton("승인");

		expect(document.body.textContent).toContain("742-918");
		expect(document.body.textContent).toContain("joon-macbook");
		expect(document.body.textContent).toContain("darwin");
		expect(document.body.textContent).toContain("arm64");
		expect(document.body.textContent).toContain("0.1.0");
		expect(calls[0]).toEqual({
			url: "/api/pair/approval/status?approvalId=approval_123",
			method: undefined,
			body: undefined,
		});
		expect(calls[1]).toEqual({
			url: "/api/pair/approval/approve",
			method: "POST",
			body: JSON.stringify({
				approvalId: "approval_123",
				nonce: "nonce_123",
				csrfToken: "csrf_123",
				decision: "approve",
			}),
		});
		expect(JSON.stringify(calls)).not.toContain("pair_");
		expect(JSON.stringify(calls)).not.toContain("machine_token");
	});

	it("stops polling on locked approval status", async () => {
		vi.useFakeTimers();
		openApprovalRoute();
		const calls: string[] = [];
		vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
			calls.push(String(input));
			return jsonResponse({
				status: "locked",
				expiresAt: "2026-06-19T08:10:00Z",
			});
		});

		await renderApp();
		await act(async () => {
			await vi.advanceTimersByTimeAsync(6000);
		});

		expect(document.body.textContent).toContain("승인 요청이 잠겼습니다");
		expect(document.body.textContent).toContain("neul login");
		expect(calls).toEqual([
			"/api/pair/approval/status?approvalId=approval_123",
		]);
	});

	it("shows neul up guidance when the approval is claimed", async () => {
		openApprovalRoute();
		vi.stubGlobal("fetch", async () =>
			jsonResponse({
				status: "claimed",
				machineId: "machine_123",
				claimedAt: "2026-06-19T08:04:00Z",
				expiresAt: "2026-06-19T08:10:00Z",
			}),
		);

		await renderApp();

		expect(document.body.textContent).toContain("neul up");
		expect(document.body.textContent).toContain("machine_123");
		expect(document.body.textContent).not.toContain("pair_");
	});
});

async function clickButton(name: string): Promise<void> {
	await act(async () => {
		getButton(name).click();
	});
}

function openApprovalRoute(): void {
	window.history.pushState(
		null,
		"",
		"/enroll/approve?approval=approval_123&nonce=nonce_123",
	);
}
