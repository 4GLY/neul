import type { ApiResource } from "./apiTypes";
import {
	errorCodeFromResponse,
	OwnerSessionRequiredError,
} from "./localSession";

export type DotfileResourceInput = {
	readonly path: string;
	readonly content: string;
	readonly mode: string;
	readonly applyMode: "copy" | "symlink";
	readonly targetSegment: string;
};

export async function createPackageResource(input: {
	readonly name: string;
	readonly sourceKind: "brew" | "apt" | "mise";
	readonly desiredVersion: string;
	readonly targetSegment: string;
}): Promise<ApiResource> {
	return sendResource("POST", "/api/resources/package", input);
}

export async function createDotfileResource(
	input: DotfileResourceInput,
): Promise<ApiResource> {
	return sendResource("POST", "/api/resources/dotfile", input);
}

export async function updateResource(
	id: string,
	input: DotfileResourceInput,
): Promise<ApiResource> {
	return sendResource(
		"PATCH",
		`/api/resources/${encodeURIComponent(id)}`,
		input,
	);
}

export async function deleteResource(id: string): Promise<void> {
	const response = await fetch(`/api/resources/${encodeURIComponent(id)}`, {
		method: "DELETE",
	});
	if (!response.ok) {
		await handleResourceError(response);
	}
}

async function sendResource<T extends object>(
	method: "POST" | "PATCH",
	path: string,
	body: T,
): Promise<ApiResource> {
	const response = await fetch(path, {
		method,
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(body),
	});
	if (!response.ok) {
		await handleResourceError(response);
	}
	return (await response.json()) as ApiResource;
}

async function handleResourceError(response: Response): Promise<never> {
	const code = await errorCodeFromResponse(response);
	if (
		response.status === 401 &&
		(code === "unauthorized" || code === "owner_session_required")
	) {
		throw new OwnerSessionRequiredError();
	}
	if (code === "path_not_allowed") {
		throw new Error("경로를 사용할 수 없습니다");
	}
	if (code === "dotfile_invalid") {
		throw new Error("모드 또는 적용 방식이 올바르지 않습니다");
	}
	throw new Error("리소스를 저장하지 못했습니다");
}
