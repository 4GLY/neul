import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ApiResource } from "./apiTypes";
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

	it("updates and deletes an existing dotfile resource", async () => {
		const calls: {
			readonly url: string;
			readonly method: string;
			readonly body: string;
		}[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push({
					url: String(input),
					method: init?.method ?? "GET",
					body: String(init?.body ?? ""),
				});
				if (init?.method === "DELETE") {
					return new Response(null, { status: 204 });
				}
				return new Response(
					JSON.stringify({
						id: "resource_dot_zshrc",
						kind: "dotfile",
						name: "~/.zshrc",
						desiredVersion: 4,
						agentSupport: "supported",
						spec: {
							path: "~/.zshrc",
							content: "export EDITOR=nvim\n",
							mode: "0600",
							applyMode: "symlink",
							targetSegment: "base",
						},
					}),
					{ status: 200, headers: { "Content-Type": "application/json" } },
				);
			},
		);

		await renderEditor({
			resources: [
				{
					id: "resource_dot_zshrc",
					kind: "dotfile",
					name: "~/.zshrc",
					desiredVersion: 3,
					agentSupport: "supported",
					spec: {
						path: "~/.zshrc",
						content: "alias ll='ls -la'\n",
						mode: "0644",
						applyMode: "copy",
						targetSegment: "base",
					},
				},
			],
		});
		await click("Dotfile");
		selectOption("Existing dotfile", "resource_dot_zshrc");
		setTextarea("Dotfile content", "export EDITOR=nvim\n");
		setInput("Dotfile mode", "0600");
		selectOption("Dotfile apply mode", "symlink");
		await click("Update dotfile");
		await click("Delete dotfile");

		expect(calls).toEqual([
			{
				url: "/api/resources/resource_dot_zshrc",
				method: "PATCH",
				body: JSON.stringify({
					path: "~/.zshrc",
					content: "export EDITOR=nvim\n",
					mode: "0600",
					applyMode: "symlink",
					targetSegment: "base",
				}),
			},
			{
				url: "/api/resources/resource_dot_zshrc",
				method: "DELETE",
				body: "",
			},
		]);
		expect(document.body.textContent).toContain("삭제했습니다");
	});

	it("uses update as the primary action for a selected dotfile", async () => {
		const calls: string[] = [];
		vi.stubGlobal(
			"fetch",
			async (input: RequestInfo | URL, init?: RequestInit) => {
				calls.push(`${init?.method ?? "GET"} ${String(input)}`);
				return new Response(
					JSON.stringify({
						id: "resource_dot_zshrc",
						kind: "dotfile",
						name: "~/.zshrc",
						desiredVersion: 2,
						agentSupport: "supported",
						spec: {
							path: "~/.zshrc",
							content: "",
							mode: "0644",
							applyMode: "copy",
						},
					}),
					{ status: 200, headers: { "Content-Type": "application/json" } },
				);
			},
		);

		await renderEditor({
			resources: [
				{
					id: "resource_dot_zshrc",
					kind: "dotfile",
					name: "~/.zshrc",
					desiredVersion: 1,
					agentSupport: "supported",
					spec: {
						path: "~/.zshrc",
						content: "",
						mode: "0644",
						applyMode: "copy",
					},
				},
			],
		});
		await click("Dotfile");
		selectOption("Existing dotfile", "resource_dot_zshrc");
		await click("Update dotfile");

		expect(calls).toEqual(["PATCH /api/resources/resource_dot_zshrc"]);
		expect(calls).not.toContain("POST /api/resources/dotfile");
	});
});

type RenderEditorOptions = {
	readonly resources?: readonly ApiResource[];
};

async function renderEditor(options: RenderEditorOptions = {}): Promise<void> {
	const rootElement = document.createElement("div");
	document.body.appendChild(rootElement);
	const root = createRoot(rootElement);
	await act(async () => {
		root.render(<ResourceEditor onSaved={() => undefined} {...options} />);
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

function selectOption(label: string, value: string): void {
	const select = document.querySelector<HTMLSelectElement>(
		`select[aria-label="${label}"]`,
	);
	if (select === null) {
		throw new Error(`select ${label} not found`);
	}
	act(() => {
		select.value = value;
		select.dispatchEvent(new Event("change", { bubbles: true }));
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
