import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
	expectQueuedCommand,
	findResource,
	postDriftReport,
	runAgentTick,
	visibleOnboardMachine,
	writeQaDoc,
} from "./mvp-flow";
import {
	evidenceDir,
	ownerSessionCookie,
	startServer,
	stopServer,
} from "./server-fixture";

test.afterEach(() => {
	stopServer.cleanDist();
});

test("fresh empty instance shows first-machine CTA", async ({ page }) => {
	const fixture = await startServer("empty");
	try {
		const cookie = await ownerSessionCookie(
			fixture.baseURL,
			fixture.setupToken,
		);
		await page.context().addCookies([cookie]);
		await page.goto(fixture.baseURL);
		await expect(
			page.getByRole("heading", { name: "첫 머신을 등록하세요" }),
		).toBeVisible();
		await page.screenshot({
			fullPage: true,
			path: join(evidenceDir, "task-15-empty-instance-browser.png"),
		});
	} finally {
		await stopServer(fixture, "task-15-empty");
	}
});

test("full MVP dashboard flow visibly onboards, edits, repairs, and agent-acks", async ({
	page,
}) => {
	test.setTimeout(75_000);
	const fixture = await startServer("mvp");
	try {
		const cookie = await ownerSessionCookie(
			fixture.baseURL,
			fixture.setupToken,
		);
		await page.context().addCookies([cookie]);

		const enrolled = await visibleOnboardMachine(page, fixture);
		await expect(page.getByText("Connected").first()).toBeVisible();
		await expect(page.getByText("No drift")).toBeVisible();

		await page.getByRole("button", { name: "Edit desired state" }).click();
		await page.getByLabel("Package name").fill("kubectl");
		await page.getByRole("button", { name: "Save package" }).click();
		await expect(
			page.locator(".ledger").getByText("kubectl").first(),
		).toBeVisible();

		const packagePatchResponsePromise = page.waitForResponse(
			(response) =>
				response.request().method() === "PATCH" &&
				response.url().includes("/api/resources/"),
		);
		await page.getByRole("button", { name: "Edit kubectl" }).click();
		await page.getByLabel("Package version").fill("1.2.3");
		await page.getByRole("button", { name: "Save package" }).click();
		expect((await packagePatchResponsePromise).status()).toBe(200);
		await expect(page.locator(".ledger").getByText("1.2.3")).toBeVisible();

		await page.getByLabel("Package name").fill("ripgrep");
		await page.getByLabel("Package version").fill("latest");
		await page.getByRole("button", { name: "Save package" }).click();
		await expect(
			page.locator(".ledger").getByText("ripgrep").first(),
		).toBeVisible();
		const deletedPackage = await findResource(
			fixture.baseURL,
			cookie.value,
			"ripgrep",
		);
		const packageDeleteResponsePromise = page.waitForResponse((response) =>
			response.url().includes(`/api/resources/${deletedPackage.id}`),
		);
		page.once("dialog", (dialog) => dialog.accept());
		await page.getByRole("button", { name: "Delete ripgrep" }).click();
		expect((await packageDeleteResponsePromise).status()).toBe(204);
		await page.reload();
		await page.getByRole("button", { name: "Edit desired state" }).click();
		await expect(page.locator(".ledger").getByText("ripgrep")).toHaveCount(0);

		await page
			.locator(".resource-editor")
			.getByRole("button", { name: "Dotfile" })
			.click();
		await page.getByLabel("Dotfile path").fill("~/.zshrc");
		await page.getByLabel("Dotfile content").fill("export NEUL_E2E=1");
		const dotfileResponsePromise = page.waitForResponse((response) =>
			response.url().includes("/api/resources/dotfile"),
		);
		await page.getByRole("button", { name: "Save dotfile" }).click();
		expect((await dotfileResponsePromise).status()).toBe(201);
		await page.reload();
		await expect(page.getByText(/\.zshrc/).first()).toBeVisible();

		const packageResource = await findResource(
			fixture.baseURL,
			cookie.value,
			"kubectl",
		);
		await postDriftReport(fixture.baseURL, enrolled, packageResource.id, 2);
		await page.reload();
		await expect(page.getByText("Connected").first()).toBeVisible();
		await expect(
			page.locator(".machine-grid.row").getByText("Drifted").first(),
		).toBeVisible();

		await page.screenshot({
			fullPage: true,
			path: join(evidenceDir, "task-6-agent-onboarding-e2e-browser.png"),
		});

		const repairResponsePromise = page.waitForResponse((response) =>
			response
				.url()
				.includes(`/api/machines/${enrolled.machineId}/repair-drift`),
		);
		await page.getByRole("button", { name: "drift 복구" }).click();
		expect((await repairResponsePromise).status()).toBe(202);
		await expect(
			page.getByText("복구 명령을 대기열에 추가했습니다"),
		).toBeVisible();

		await expectQueuedCommand(fixture.baseURL, enrolled, true);
		await runAgentTick(enrolled);
		await expectQueuedCommand(fixture.baseURL, enrolled, false);

		await page.getByRole("button", { name: "로그 열기" }).click();
		await expect(page.getByText("dry_run_queued")).toBeVisible();

		writeQaDoc(fixture, enrolled);
	} finally {
		await stopServer(fixture, "task-6-agent-onboarding");
	}
});
