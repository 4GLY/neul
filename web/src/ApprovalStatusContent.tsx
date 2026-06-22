import type { ReactElement } from "react";
import type { ApiApprovalStatusResponse } from "./apiTypes";

export function ApprovalStatusContent({
	status,
	onApprove,
	onCancel,
}: {
	readonly status: ApiApprovalStatusResponse;
	readonly onApprove: () => void;
	readonly onCancel: () => void;
}): ReactElement {
	switch (status.status) {
		case "pending":
		case "approved":
			return (
				<>
					<p>
						터미널의 비교 코드와 아래 코드가 같을 때만 승인하세요. 다른 사람이나
						채팅에서 받은 승인 URL은 승인하지 마세요.
					</p>
					<strong className="approval-code">{status.comparisonCode}</strong>
					<dl className="approval-machine">
						<div>
							<dt>Machine</dt>
							<dd>{status.machine.name}</dd>
						</div>
						<div>
							<dt>OS</dt>
							<dd>{status.machine.os}</dd>
						</div>
						<div>
							<dt>Architecture</dt>
							<dd>{status.machine.arch}</dd>
						</div>
						<div>
							<dt>Agent</dt>
							<dd>{status.machine.agentVersion}</dd>
						</div>
						{status.requestedAt === undefined ? null : (
							<div>
								<dt>Requested</dt>
								<dd>{formatApprovalTime(status.requestedAt)}</dd>
							</div>
						)}
						<div>
							<dt>Expires</dt>
							<dd>{formatApprovalTime(status.expiresAt)}</dd>
						</div>
					</dl>
					<div className="actions">
						<button
							className="primary-button"
							type="button"
							onClick={onApprove}
						>
							승인
						</button>
						<button
							className="secondary-button"
							type="button"
							onClick={onCancel}
						>
							취소
						</button>
					</div>
				</>
			);
		case "claimed":
			return (
				<>
					<p>로그인이 완료되었습니다. 이 machine을 계속 연결하려면 실행:</p>
					<code>neul up</code>
					<p>Machine ID: {status.machineId}</p>
					<p>Claimed: {formatApprovalTime(status.claimedAt)}</p>
				</>
			);
		case "expired":
			return (
				<p>
					승인 시간이 만료되었습니다. 터미널에서 neul login을 다시 실행하세요.
				</p>
			);
		case "cancelled":
			return (
				<p>승인이 취소되었습니다. 필요하면 neul login을 다시 실행하세요.</p>
			);
		case "locked":
			return (
				<p>
					승인 요청이 잠겼습니다. pair code는 발급되지 않았습니다. 터미널에서
					neul login을 다시 실행하세요.
				</p>
			);
	}
}

function formatApprovalTime(value: string): string {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) {
		return value;
	}
	const iso = date.toISOString();
	return `${iso.slice(0, 10)} ${iso.slice(11, 16)} UTC`;
}
