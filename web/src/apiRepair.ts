import {
	errorCodeFromResponse,
	OwnerSessionRequiredError,
} from "./localSession";

export async function repairDrift(
	machineId: string,
	resourceIds: readonly string[] = [],
): Promise<void> {
	const response = await fetch(`/api/machines/${machineId}/repair-drift`, {
		method: "POST",
		headers: { "Idempotency-Key": `web-repair-${machineId}-${Date.now()}` },
		body: JSON.stringify({ resourceIds }),
	});
	if (!response.ok) {
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
		throw new Error("drift 복구 명령을 만들지 못했습니다");
	}
}

async function isOwnerSessionRequiredResponse(
	response: Response,
): Promise<boolean> {
	if (response.status !== 401) {
		return false;
	}
	const code = await errorCodeFromResponse(response);
	return code === "unauthorized" || code === "owner_session_required";
}
