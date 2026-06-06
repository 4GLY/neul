import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ResourceEditor } from "./ResourceEditor";

describe("ResourceEditor", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		document.body.innerHTML = "";
	});

	it("creates a package through the resources API", async () => {
		const calls: string[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push(
					`${init?.method ?? "GET"} ${String(input)} ${String(init?.body)}`,
				);
				return new Response(
					JSON.stringify({
						id: "resource_1",
						kind: "package",
						name: "kubectl",
						desiredVersion: 1,
						agentSupport: "supported",
						spec: { sourceKind: "brew", desiredVersion: "latest" },
					}),
					{ status: 201, headers: { "Content-Type": "application/json" } },
				);
			},
		);

		await renderEditor();
		setInput("Package name", "kubectl");
		setInput("Package version", "latest");
		await click("Save package");

		expect(
			calls.some((call) => call.includes("POST /api/resources/package")),
		).toBe(true);
		expect(document.body.textContent).toContain("저장했습니다");
		expect(document.body.textContent).not.toContain("secret");
	});

	it("shows a Korean server error for hostile dotfile paths", async () => {
		vi.stubGlobal(
			"fetch",
			async () =>
				new Response(
					JSON.stringify({
						error: {
							code: "path_not_allowed",
							message: "Dotfile path is not allowed.",
						},
					}),
					{ status: 400, headers: { "Content-Type": "application/json" } },
				),
		);

		await renderEditor();
		await click("Dotfile");
		setInput("Dotfile path", "/etc/hosts");
		setTextarea("Dotfile content", "x");
		await click("Save dotfile");

		expect(document.body.textContent).toContain("경로를 사용할 수 없습니다");
		expect(document.body.textContent).not.toContain("저장했습니다");
	});
});

async function renderEditor(): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<ResourceEditor onSaved={() => undefined} />);
	});
}

function setInput(label: string, value: string): void {
	const input = document.querySelector<HTMLInputElement>(
		`input[aria-label="${label}"]`,
	);
	if (input === null) {
		throw new Error(`input ${label} not found`);
	}
	act(() => {
		input.value = value;
		input.dispatchEvent(new Event("input", { bubbles: true }));
	});
}

function setTextarea(label: string, value: string): void {
	const textarea = document.querySelector<HTMLTextAreaElement>(
		`textarea[aria-label="${label}"]`,
	);
	if (textarea === null) {
		throw new Error(`textarea ${label} not found`);
	}
	act(() => {
		textarea.value = value;
		textarea.dispatchEvent(new Event("input", { bubbles: true }));
	});
}

async function click(name: string): Promise<void> {
	const button = [...document.querySelectorAll("button")].find(
		(item) => item.textContent === name,
	);
	if (button === undefined) {
		throw new Error(`button ${name} not found`);
	}
	await act(async () => {
		button.dispatchEvent(new MouseEvent("click", { bubbles: true }));
	});
}
