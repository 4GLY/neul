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
			body: "이 브라우저에서 등록 명령을 만들고, neul checkout에서 한 번 실행하면 agent 연결을 확인합니다.",
			action: "첫 머신 등록",
		});
		expect(copy.onboarding).toMatchObject({
			title: "머신 등록",
			commandReady: "명령 실행 대기 중",
			checkingAgent: "agent 연결 확인 중",
			agentNotResponding: "agent 응답 없음",
			connected: "연결됨",
			checkoutHint: "Run from your neul checkout:",
		});
	});

	it("documents pair-token handling as a browser leak guardrail", () => {
		expect(copy.onboarding.security.pairTokenKind).toBe("bearer");
		expect(copy.onboarding.security.neverStorePairTokenIn).toEqual([
			"URL query strings",
			"document.title",
			"browser history",
		]);
	});
});
