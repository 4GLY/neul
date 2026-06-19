import type {
	ApiApprovalDecisionRequest,
	ApiApprovalDecisionResponse,
	ApiApprovalStatusResponse,
} from "./apiTypes";
import {
	errorCodeFromResponse,
	OwnerSessionRequiredError,
} from "./localSession";

export async function loadApprovalStatus(
	approvalId: string,
): Promise<ApiApprovalStatusResponse> {
	return fetchApprovalJSON<ApiApprovalStatusResponse>(
		`/api/pair/approval/status?approvalId=${encodeURIComponent(approvalId)}`,
	);
}

export async function submitApprovalDecision(
	request: ApiApprovalDecisionRequest,
): Promise<ApiApprovalDecisionResponse> {
	const response = await fetch("/api/pair/approval/approve", {
		method: "POST",
		credentials: "same-origin",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(request),
	});
	if (!response.ok) {
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
		throw new Error("승인 상태를 변경하지 못했습니다");
	}
	return (await response.json()) as ApiApprovalDecisionResponse;
}

async function fetchApprovalJSON<T>(path: string): Promise<T> {
	const response = await fetch(path);
	if (!response.ok) {
		if (await isOwnerSessionRequiredResponse(response)) {
			throw new OwnerSessionRequiredError();
		}
		throw new Error("승인 요청을 불러오지 못했습니다");
	}
	return (await response.json()) as T;
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
