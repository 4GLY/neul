import type { ReactElement } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ApprovalStatusContent } from "./ApprovalStatusContent";
import { loadApprovalStatus, submitApprovalDecision } from "./apiApproval";
import type { ApiApprovalStatusResponse } from "./apiTypes";
import { OwnerSessionRequiredError } from "./localSession";
import { StatePanel } from "./StatePanel";

const approvalPollMs = 2000;

type ApprovalRouteParams =
	| {
			readonly kind: "valid";
			readonly approvalId: string;
			readonly nonce: string;
	  }
	| { readonly kind: "invalid" };

type ApprovalPageState =
	| { readonly kind: "loading" }
	| { readonly kind: "owner_session_required" }
	| { readonly kind: "ready"; readonly status: ApiApprovalStatusResponse }
	| { readonly kind: "error"; readonly message: string };

export function ApprovalPage(): ReactElement {
	const routeParams = useMemo(readApprovalRouteParams, []);
	const [state, setState] = useState<ApprovalPageState>({ kind: "loading" });

	const refreshStatus = useCallback(async (): Promise<void> => {
		if (routeParams.kind === "invalid") {
			setState({
				kind: "error",
				message: "승인 링크가 올바르지 않습니다. neul login을 다시 실행하세요.",
			});
			return;
		}
		try {
			const status = await loadApprovalStatus(routeParams.approvalId);
			setState({ kind: "ready", status });
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				setState({ kind: "owner_session_required" });
				return;
			}
			setState({
				kind: "error",
				message:
					error instanceof Error
						? error.message
						: "승인 요청을 불러오지 못했습니다.",
			});
		}
	}, [routeParams]);

	useEffect(() => {
		void refreshStatus();
	}, [refreshStatus]);

	useEffect(() => {
		if (state.kind !== "ready") {
			return;
		}
		if (isTerminalStatus(state.status.status)) {
			return;
		}
		const intervalId = window.setInterval(() => {
			void refreshStatus();
		}, approvalPollMs);
		return () => {
			window.clearInterval(intervalId);
		};
	}, [refreshStatus, state]);

	const submitDecision = useCallback(
		async (decision: "approve" | "cancel"): Promise<void> => {
			if (routeParams.kind === "invalid" || state.kind !== "ready") {
				return;
			}
			const status = state.status;
			if (status.status !== "pending" && status.status !== "approved") {
				return;
			}
			try {
				await submitApprovalDecision({
					approvalId: status.approvalId,
					nonce: routeParams.nonce,
					csrfToken: status.csrfToken,
					decision,
				});
				await refreshStatus();
			} catch (error) {
				if (error instanceof OwnerSessionRequiredError) {
					setState({ kind: "owner_session_required" });
					return;
				}
				setState({
					kind: "error",
					message:
						error instanceof Error
							? error.message
							: "승인 상태를 변경하지 못했습니다.",
				});
			}
		},
		[refreshStatus, routeParams, state],
	);

	if (state.kind === "loading") {
		return (
			<StatePanel
				title="Neul 로그인 승인"
				body="승인 요청을 확인하고 있습니다."
			/>
		);
	}

	if (state.kind === "owner_session_required") {
		return (
			<StatePanel
				title="owner session이 필요합니다"
				body="이 브라우저에는 owner session이 없습니다. 같은 승인 URL을 이미 로그인된 owner 브라우저에서 열거나, 이 브라우저에서 owner session을 먼저 만든 뒤 다시 시도하세요."
			/>
		);
	}

	if (state.kind === "error") {
		return <StatePanel title="승인 요청 오류" body={state.message} />;
	}

	return (
		<main className="approval-page" aria-label="Neul 로그인 승인">
			<section className="state-panel">
				<h2>Neul 로그인 승인</h2>
				<ApprovalStatusContent
					status={state.status}
					onApprove={() => {
						void submitDecision("approve");
					}}
					onCancel={() => {
						void submitDecision("cancel");
					}}
				/>
			</section>
		</main>
	);
}

function readApprovalRouteParams(): ApprovalRouteParams {
	const params = new URLSearchParams(window.location.search);
	const approvalId = params.get("approval");
	const nonce = params.get("nonce");
	if (
		approvalId === null ||
		approvalId === "" ||
		nonce === null ||
		nonce === ""
	) {
		return { kind: "invalid" };
	}
	return { kind: "valid", approvalId, nonce };
}

function isTerminalStatus(
	status: ApiApprovalStatusResponse["status"],
): boolean {
	return (
		status === "claimed" ||
		status === "expired" ||
		status === "cancelled" ||
		status === "locked"
	);
}
