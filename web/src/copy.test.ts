import { describe, expect, it } from "vitest";
import { copy } from "./copy";

describe("Korean-first copy", () => {
	it("keeps only the explicit English allowlist categories", () => {
		expect([...copy.englishAllowlist]).toEqual([
			"CLI commands",
			"package names",
			"API paths",
			"protocol fields",
			"OS names",
			"semantic status enum values",
		]);
	});

	it("uses Korean labels for the main dashboard actions", () => {
		expect(copy.dashboard.pageTitle).toBe("머신");
		expect(copy.dashboard.reconcileNow).toBe("지금 reconcile");
		expect(copy.inspector.repairDrift).toBe("drift 복구");
		expect(copy.inspector.openLogs).toBe("로그 열기");
		expect(copy.dashboard.pageTitle).not.toBe("Machines");
		expect(copy.inspector.repairDrift).not.toBe("Repair drift");
	});

	it("defines the agent onboarding v2 copy contract", () => {
		expect(copy.dashboard.emptyState).toEqual({
			title: "첫 머신을 등록하세요",
			body: "packaged neul client에서 로그인한 뒤 neul up으로 agent 연결을 확인합니다.",
			action: "첫 머신 등록",
		});
		expect(copy.onboarding).toMatchObject({
			title: "머신 등록",
			intro:
				"로그인 명령을 만든 뒤 새 머신에서 실행하고, 이어 neul up으로 연결을 확인하세요.",
			createInvite: "로그인 명령 만들기",
			commandReady: "명령 실행 대기 중",
			checkingAgent: "agent 연결 확인 중",
			agentNotResponding: "agent 응답 없음",
			connected: "연결됨",
			checkoutHint: "Run with packaged neul client:",
			fallbackHint:
				"packaged approval flow가 준비되기 전에는 fallback/debug 명령으로 등록하세요:",
		});
		expect(copy.onboarding.installOptions).toEqual([
			"macOS local QA: unsigned dev .pkg",
			"Production macOS: Developer ID Application/Installer, notarization, stapling",
			"Linux: Debian/Ubuntu .deb 또는 tarball",
		]);
		expect(copy.onboarding.commandTemplate).toBe(
			"neul login --server <origin>",
		);
		expect(copy.onboarding.fallbackCommandTemplate).toBe(
			"go run ./cmd/neul agent enroll --server <origin> --pair <pair-code> --connect-once",
		);
	});

	it("documents pair-code handling as a browser leak guardrail", () => {
		expect(copy.onboarding.security.pairCodeKind).toContain("/api/pair/claim");
		expect(copy.onboarding.security.browserExcludedCredentials).toEqual([
			"pair code",
			"pair token",
			"machine token",
			"setup token",
			"plaintext verifier",
		]);
		expect(copy.onboarding.security.neverStorePairCodeIn).toEqual([
			"browser copy",
			"URLs",
			"document.title",
			"browser history",
			"localStorage",
			"logs",
		]);
		expect(copy.onboarding.security.browserSafeApprovalHandoffs).toEqual([
			"approval id",
			"nonce",
			"comparison code",
			"machine preview metadata",
			"CSRF",
			"status",
		]);
		expect(JSON.stringify(copy)).not.toContain("allowedPairTokenHandoffs");
	});
});
