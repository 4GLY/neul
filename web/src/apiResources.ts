import type { ApiResource } from "./apiTypes";
import {
	errorCodeFromResponse,
	OwnerSessionRequiredError,
} from "./localSession";

type PackageResourceInput = {
	readonly name: string;
	readonly sourceKind: "brew" | "apt" | "mise";
	readonly desiredVersion: string;
};

export type DotfileResourceInput = {
	readonly path: string;
	readonly content: string;
	readonly mode: string;
	readonly applyMode: "copy" | "symlink";
	readonly targetSegment: string;
};

export type CreatePackageResourceInput = PackageResourceInput & {
	readonly targetSegment: string;
};

export async function createPackageResource(
	input: CreatePackageResourceInput,
): Promise<ApiResource> {
	return writeResource("/api/resources/package", "POST", input);
}

export async function updatePackageResource(
	resourceId: string,
	input: PackageResourceInput,
): Promise<ApiResource> {
	return writeResource(`/api/resources/${resourceId}`, "PATCH", input);
}

export async function createDotfileResource(input: {
	readonly path: string;
	readonly content: string;
	readonly mode: string;
	readonly applyMode: "copy" | "symlink";
	readonly targetSegment: string;
}): Promise<ApiResource> {
	return writeResource("/api/resources/dotfile", "POST", input);
}

export async function updateResource(
	id: string,
	input: DotfileResourceInput,
): Promise<ApiResource> {
	return writeResource(
		`/api/resources/${encodeURIComponent(id)}`,
		"PATCH",
		input,
	);
}

export async function deleteResource(resourceId: string): Promise<void> {
	const response = await fetch(
		`/api/resources/${encodeURIComponent(resourceId)}`,
		{
			method: "DELETE",
		},
	);
	if (!response.ok) {
		await handleResourceWriteError(response);
	}
}

async function writeResource<T extends object>(
	path: string,
	method: "PATCH" | "POST",
	body: T,
): Promise<ApiResource> {
	const response = await fetch(path, {
		method,
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(body),
	});
	if (!response.ok) {
		await handleResourceWriteError(response);
	}
	return (await response.json()) as ApiResource;
}

async function handleResourceWriteError(response: Response): Promise<never> {
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
