import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import {
	appendActions,
	assertNoTokenLeak,
	cleanupReceipt,
	expectNoSessionCookie,
	watchLeakSurfaces,
} from "./owner-setup-helpers";
import {
	ownerSessionCookie,
	repoRoot,
	startServer,
	stopServer,
} from "./server-fixture";

const ulwEvidenceDir =
	process.env.ULW_EVIDENCE_DIR ?? join(repoRoot, "evidence", "owner-setup");

test.afterEach(() => {
	stopServer.cleanDist();
});

test("first-run setup exchanges token and lands on dashboard", async ({
	page,
}) => {
	mkdirSync(ulwEvidenceDir, { recursive: true });
	const fixture = await startServer("owner-setup-happy");
	const leakSurfaces = watchLeakSurfaces(page);
	const actions: string[] = [
		"channel=browser use",
		`goto=${fixture.baseURL}/`,
		"pass=setup screen visible, local session 204, dashboard visible",
	];
	try {
		await page.goto(fixture.baseURL);
		await expect(
			page.getByRole("heading", { name: "첫 실행 설정" }),
		).toBeVisible();
		await assertNoTokenLeak(page, "setup_");
		await page.getByLabel("setup token").fill(fixture.setupToken);
		const sessionResponse = page.waitForResponse((response) =>
			response.url().endsWith("/api/session/local"),
		);
		await page.getByRole("button", { name: "설정 완료" }).click();
		expect((await sessionResponse).status()).toBe(204);
		await expect(
			page.getByRole("heading", { exact: true, name: "머신" }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "첫 머신 등록" }),
		).toBeVisible();
		const cookies = await page.context().cookies(fixture.baseURL);
		expect(cookies.some((cookie) => cookie.name === "neul_session")).toBe(true);
		await assertNoTokenLeak(page, fixture.setupToken, leakSurfaces);
		await page.screenshot({
			fullPage: true,
			path: join(ulwEvidenceDir, "C001-happy-dashboard.png"),
		});
	} finally {
		await stopServer(fixture, "C001-owner-setup");
		actions.push(cleanupReceipt(fixture));
		writeFileSync(
			join(ulwEvidenceDir, "C001-happy-browser.md"),
			actions.join("\n"),
		);
	}
});

test("setup form shows invalid, used, and expired token states", async ({
	page,
}) => {
	mkdirSync(ulwEvidenceDir, { recursive: true });
	const c002Log = "C002-token-states-browser.md";
	rmSync(join(ulwEvidenceDir, c002Log), { force: true });
	const invalidUsed = await startServer("owner-setup-token-states");
	const leakSurfaces = watchLeakSurfaces(page);
	const invalidUsedActions: string[] = [
		"channel=browser use",
		"invalid=fill wrong token and expect setup_token_invalid copy",
		"used=consume token over API, fill same token in browser, expect setup_token_used copy",
	];
	try {
		await page.goto(invalidUsed.baseURL);
		await page.getByLabel("setup token").fill("wrong-token");
		await page.getByRole("button", { name: "설정 완료" }).click();
		await expect(
			page.getByText("setup token이 올바르지 않습니다."),
		).toBeVisible();
		await expectNoSessionCookie(page, invalidUsed.baseURL);
		await page.screenshot({
			fullPage: true,
			path: join(ulwEvidenceDir, "C002-invalid.png"),
		});

		await ownerSessionCookie(invalidUsed.baseURL, invalidUsed.setupToken);
		await page.getByLabel("setup token").fill(invalidUsed.setupToken);
		await page.getByRole("button", { name: "설정 완료" }).click();
		await expect(
			page.getByText("이미 사용된 setup token입니다."),
		).toBeVisible();
		await expectNoSessionCookie(page, invalidUsed.baseURL);
		await assertNoTokenLeak(page, invalidUsed.setupToken, leakSurfaces);
		await page.screenshot({
			fullPage: true,
			path: join(ulwEvidenceDir, "C002-used.png"),
		});
	} finally {
		await stopServer(invalidUsed, "C002-invalid-used");
		invalidUsedActions.push(cleanupReceipt(invalidUsed));
		appendActions(ulwEvidenceDir, c002Log, invalidUsedActions);
	}

	const expired = await startServer("owner-setup-expired", {
		setupTokenTTL: "1ms",
	});
	const expiredActions: string[] = [
		"expired=wait for 1ms TTL, fill token, expect setup_token_expired copy",
	];
	try {
		await page.waitForTimeout(10);
		await page.goto(expired.baseURL);
		await page.getByLabel("setup token").fill(expired.setupToken);
		await page.getByRole("button", { name: "설정 완료" }).click();
		await expect(
			page.getByText(
				"setup token이 만료되었습니다. 서버 콘솔에 새 setup token을 출력했습니다.",
			),
		).toBeVisible();
		await expectNoSessionCookie(page, expired.baseURL);
		await assertNoTokenLeak(page, expired.setupToken, leakSurfaces);
		await page.screenshot({
			fullPage: true,
			path: join(ulwEvidenceDir, "C002-expired.png"),
		});
	} finally {
		await stopServer(expired, "C002-expired");
		expiredActions.push(cleanupReceipt(expired));
		appendActions(ulwEvidenceDir, c002Log, expiredActions);
	}
});

test("authenticated dashboard and pair onboarding remain setup-token free", async ({
	page,
}) => {
	mkdirSync(ulwEvidenceDir, { recursive: true });
	const fixture = await startServer("owner-setup-regression");
	const leakSurfaces = watchLeakSurfaces(page);
	const actions: string[] = [
		"channel=browser use",
		"goto authenticated dashboard",
		"pass=empty dashboard CTA visible and pair command excludes setup token",
	];
	try {
		const cookie = await ownerSessionCookie(
			fixture.baseURL,
			fixture.setupToken,
		);
		await page.context().addCookies([cookie]);
		await page.goto(fixture.baseURL);
		await expect(
			page.getByRole("button", { name: "첫 머신 등록" }),
		).toBeVisible();
		await page.getByRole("button", { name: "첫 머신 등록" }).click();
		await page.getByText("Run from your neul checkout:").waitFor();
		const command = await page.locator("code").first().textContent();
		expect(command).not.toContain("setup_");
		await assertNoTokenLeak(page, fixture.setupToken, leakSurfaces);
		await page.screenshot({
			fullPage: true,
			path: join(ulwEvidenceDir, "C003-dashboard.png"),
		});
	} finally {
		await stopServer(fixture, "C003-regression");
		actions.push(cleanupReceipt(fixture));
		writeFileSync(
			join(ulwEvidenceDir, "C003-regression-browser.md"),
			actions.join("\n"),
		);
	}
});
