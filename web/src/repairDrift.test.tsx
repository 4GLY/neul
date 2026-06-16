import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	click,
	flushApp,
	type RepairCall,
	renderApp,
	stubDashboardFetch,
	stubPolledRepairFetch,
	stubSelectableRepairFetch,
} from "./repairDriftTestHarness";

describe("repair drift UX", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
		document.body.innerHTML = "";
	});

	it("posts repair drift for the selected machine", async () => {
		const calls: string[] = [];
		stubDashboardFetch(calls);
		await renderApp();

		await click("drift 복구");

		expect(calls).toContain("POST /api/machines/machine_1/repair-drift");
		expect(document.body.textContent).toContain("복구 명령 대기 중");
	});

	it("repairs a selected drifted resource and shows the success outcome", async () => {
		const calls: readonly RepairCall[] = stubSelectableRepairFetch("healthy");
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(calls).toContainEqual({
			url: "/api/machines/machine_1/repair-drift",
			method: "POST",
			body: JSON.stringify({ resourceIds: ["resource_brew"] }),
		});
		expect(document.body.textContent).toContain("복구 성공");
	});

	it("shows the blocked repair outcome after a repair refresh reports blocked", async () => {
		stubSelectableRepairFetch("blocked");
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(document.body.textContent).toContain("복구 차단됨");
	});

	it("keeps pending visible until a later repair poll reports success", async () => {
		vi.useFakeTimers();
		stubPolledRepairFetch();
		await renderApp();

		await click("로그 열기");
		await click("resource_brew");
		await click("drift 복구");
		await flushApp();

		expect(document.body.textContent).toContain("복구 명령 대기 중");
		expect(document.body.textContent).not.toContain("복구 성공");

		await act(async () => {
			await vi.advanceTimersByTimeAsync(5000);
		});
		await flushApp();

		expect(document.body.textContent).toContain("복구 성공");
	});

	it("opens recent event logs without streaming", async () => {
		const calls: string[] = [];
		stubDashboardFetch(calls);
		await renderApp();

		await click("로그 열기");

		expect(calls).toContain("GET /api/machines/machine_1");
		expect(document.body.textContent).toContain("kubectl missing");
	});
});
