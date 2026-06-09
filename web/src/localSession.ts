export type LocalSessionErrorCode =
	| "setup_token_required"
	| "setup_token_invalid"
	| "setup_token_used"
	| "setup_token_expired"
	| "owner_not_bootstrapped"
	| "local_session_failed";

export class LocalSessionError extends Error {
	readonly code: LocalSessionErrorCode;

	constructor(code: LocalSessionErrorCode, message: string) {
		super(message);
		this.name = "LocalSessionError";
		this.code = code;
	}
}

export class OwnerSessionRequiredError extends Error {
	constructor() {
		super("Owner session is required.");
		this.name = "OwnerSessionRequiredError";
	}
}

export async function createLocalSession(setupToken: string): Promise<void> {
	const response = await fetch("/api/session/local", {
		method: "POST",
		credentials: "same-origin",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ setupToken }),
	});
	if (response.ok) {
		return;
	}
	const code = localSessionErrorCode(await errorCodeFromResponse(response));
	throw new LocalSessionError(code, localSessionErrorMessage(code));
}

export async function errorCodeFromResponse(
	response: Response,
): Promise<string | undefined> {
	return errorCodeFromBody(await readJSONBody(response));
}

async function readJSONBody(response: Response): Promise<unknown> {
	const text = await response.text();
	if (text === "") {
		return null;
	}
	try {
		const parsed: unknown = JSON.parse(text);
		return parsed;
	} catch (error) {
		if (error instanceof SyntaxError) {
			return null;
		}
		throw error;
	}
}

function errorCodeFromBody(body: unknown): string | undefined {
	if (!hasErrorBody(body)) {
		return undefined;
	}
	const error = body.error;
	if (!hasErrorCode(error)) {
		return undefined;
	}
	const code = error.code;
	return typeof code === "string" ? code : undefined;
}

function hasErrorBody(value: unknown): value is { readonly error?: unknown } {
	return typeof value === "object" && value !== null;
}

function hasErrorCode(value: unknown): value is { readonly code?: unknown } {
	return typeof value === "object" && value !== null;
}

function localSessionErrorCode(
	code: string | undefined,
): LocalSessionErrorCode {
	switch (code) {
		case "setup_token_required":
		case "setup_token_invalid":
		case "setup_token_used":
		case "setup_token_expired":
		case "owner_not_bootstrapped":
			return code;
		default:
			return "local_session_failed";
	}
}

function localSessionErrorMessage(code: LocalSessionErrorCode): string {
	switch (code) {
		case "setup_token_required":
			return "setup token을 입력하세요.";
		case "setup_token_invalid":
			return "setup token이 올바르지 않습니다.";
		case "setup_token_used":
			return "이미 사용된 setup token입니다.";
		case "setup_token_expired":
			return "setup token이 만료되었습니다. 서버 콘솔에 새 setup token을 출력했습니다.";
		case "owner_not_bootstrapped":
			return "owner setup이 아직 준비되지 않았습니다.";
		case "local_session_failed":
			return "setup token을 확인하지 못했습니다.";
	}
}
