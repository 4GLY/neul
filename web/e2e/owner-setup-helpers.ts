import { appendFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import type { ServerFixture } from "./server-fixture";

export type LeakSurfaces = {
	readonly observed: string[];
	readonly pendingResponses: Promise<void>[];
};

export function watchLeakSurfaces(page: Page): LeakSurfaces {
	const observed: string[] = [];
	const pendingResponses: Promise<void>[] = [];
	page.on("console", (message) => {
		observed.push(`console:${message.type()}:${message.text()}`);
	});
	page.on("request", (request) => {
		observed.push(`request:${request.method()} ${request.url()}`);
	});
	page.on("response", (response) => {
		const contentType = response.headers()["content-type"] ?? "";
		if (!contentType.includes("json") && !contentType.includes("text")) {
			return;
		}
		const pending = response
			.text()
			.then((text) => {
				observed.push(`response:${response.url()}:${text}`);
			})
			.catch(() => undefined);
		pendingResponses.push(pending);
	});
	return { observed, pendingResponses };
}

export async function assertNoTokenLeak(
	page: Page,
	token: string,
	leakSurfaces?: LeakSurfaces,
): Promise<void> {
	expect(page.url()).not.toContain(token);
	expect(await page.title()).not.toContain(token);
	const surfaces = await page.evaluate(() => ({
		body: document.body.innerHTML,
		historyPath: window.location.href,
		inputValues: Array.from(document.querySelectorAll("input")).map(
			(input) => input.value,
		),
		localStorageValues: Object.values(window.localStorage),
		sessionStorageValues: Object.values(window.sessionStorage),
	}));
	expect(surfaces.body).not.toContain(token);
	expect(surfaces.historyPath).not.toContain(token);
	expect(surfaces.inputValues.join("\n")).not.toContain(token);
	expect(surfaces.localStorageValues.join("\n")).not.toContain(token);
	expect(surfaces.sessionStorageValues.join("\n")).not.toContain(token);
	if (leakSurfaces !== undefined) {
		await Promise.all(leakSurfaces.pendingResponses);
		expect(leakSurfaces.observed.join("\n")).not.toContain(token);
	}
}

export async function expectNoSessionCookie(
	page: Page,
	baseURL: string,
): Promise<void> {
	const cookies = await page.context().cookies(baseURL);
	expect(cookies.some((cookie) => cookie.name === "neul_session")).toBe(false);
}

export function cleanupReceipt(fixture: ServerFixture): string {
	return [
		"cleanup:",
		`process_exited=${fixture.process.exitCode !== null || fixture.process.signalCode !== null}`,
		`db_removed=${!existsSync(fixture.dbPath)}`,
		`temp_removed=${!existsSync(fixture.tempDir)}`,
	].join(" ");
}

export function appendActions(
	evidenceDir: string,
	filename: string,
	actions: readonly string[],
): void {
	appendFileSync(join(evidenceDir, filename), `${actions.join("\n")}\n`);
}
