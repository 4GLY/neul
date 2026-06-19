import type { ReactElement } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
	createPairingInvite,
	loadDashboardData,
	OwnerSessionRequiredError,
	pollPairingInvite,
} from "./api";
import type { ApiPairInitResponse } from "./apiTypes";
import { copy } from "./copy";
import { StatePanel } from "./StatePanel";

const pairPollMs = 2000;
const heartbeatPollMs = 2000;

type WizardState =
	| { readonly kind: "creating" }
	| { readonly kind: "ready"; readonly invite: ApiPairInitResponse }
	| {
			readonly kind: "claimed_waiting_heartbeat";
			readonly invite: ApiPairInitResponse;
			readonly machineId: string;
	  }
	| { readonly kind: "connected" }
	| { readonly kind: "expired"; readonly expiresAt: string }
	| { readonly kind: "error"; readonly message: string };

export function OnboardingWizard({
	onClose,
	onConnected,
	onOwnerSessionRequired,
}: {
	readonly onClose: () => void;
	readonly onConnected: () => void;
	readonly onOwnerSessionRequired?: () => void;
}): ReactElement {
	const [state, setState] = useState<WizardState>({ kind: "creating" });

	const createInvite = useCallback(async (): Promise<void> => {
		setState({ kind: "creating" });
		try {
			const invite = await createPairingInvite();
			setState({ kind: "ready", invite });
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				onOwnerSessionRequired?.();
				return;
			}
			setState({
				kind: "error",
				message:
					error instanceof Error
						? error.message
						: "등록 명령을 만들지 못했습니다.",
			});
		}
	}, [onOwnerSessionRequired]);

	useEffect(() => {
		void createInvite();
	}, [createInvite]);

	useEffect(() => {
		if (state.kind !== "ready") {
			return;
		}
		const intervalId = window.setInterval(() => {
			void pollPairingInvite(state.invite.code)
				.then((result) => {
					if (result.status === "claimed") {
						setState({
							kind: "claimed_waiting_heartbeat",
							invite: state.invite,
							machineId: result.machineId,
						});
						return;
					}
					if (result.status === "expired") {
						setState({ kind: "expired", expiresAt: result.expiresAt });
					}
				})
				.catch((error: unknown) => {
					if (error instanceof OwnerSessionRequiredError) {
						onOwnerSessionRequired?.();
						return;
					}
					setState({
						kind: "error",
						message:
							error instanceof Error
								? error.message
								: "등록 상태를 확인하지 못했습니다.",
					});
				});
		}, pairPollMs);
		return () => {
			window.clearInterval(intervalId);
		};
	}, [onOwnerSessionRequired, state]);

	useEffect(() => {
		if (state.kind !== "claimed_waiting_heartbeat") {
			return;
		}
		const intervalId = window.setInterval(() => {
			void loadDashboardData()
				.then((dashboard) => {
					const connected = dashboard.machines.some(
						(machine) =>
							machine.id === state.machineId && machine.lastSeen !== "unknown",
					);
					if (connected) {
						setState({ kind: "connected" });
						onConnected();
					}
				})
				.catch((error: unknown) => {
					if (error instanceof OwnerSessionRequiredError) {
						onOwnerSessionRequired?.();
						return;
					}
					setState({
						kind: "error",
						message:
							error instanceof Error
								? error.message
								: "agent 연결을 확인하지 못했습니다.",
					});
				});
		}, heartbeatPollMs);
		return () => {
			window.clearInterval(intervalId);
		};
	}, [onConnected, onOwnerSessionRequired, state]);

	const primaryCommand = useMemo(() => {
		if (state.kind !== "ready" && state.kind !== "claimed_waiting_heartbeat") {
			return "";
		}
		return `neul login --server ${window.location.origin}`;
	}, [state]);

	const fallbackCommand = useMemo(() => {
		if (state.kind !== "ready" && state.kind !== "claimed_waiting_heartbeat") {
			return "";
		}
		return `go run ./cmd/neul agent enroll --server ${window.location.origin} --pair ${state.invite.code} --connect-once`;
	}, [state]);

	if (state.kind === "creating") {
		return (
			<StatePanel
				title={copy.onboarding.title}
				body="등록 명령을 만들고 있습니다."
			/>
		);
	}

	if (state.kind === "ready") {
		return (
			<section className="state-panel" aria-label={copy.onboarding.title}>
				<h2>{copy.onboarding.commandReady}</h2>
				<ul>
					{copy.onboarding.installOptions.map((option) => (
						<li key={option}>{option}</li>
					))}
				</ul>
				<p>{copy.onboarding.checkoutHint}</p>
				<code>{primaryCommand}</code>
				<p>{copy.onboarding.fallbackHint}</p>
				<code>{fallbackCommand}</code>
				<div className="actions">
					<button className="secondary-button" type="button" onClick={onClose}>
						취소
					</button>
				</div>
			</section>
		);
	}

	if (state.kind === "claimed_waiting_heartbeat") {
		return (
			<StatePanel
				title={copy.onboarding.checkingAgent}
				body="등록은 승인되었습니다. 이 머신에서 neul up을 실행하거나 상태를 확인하세요. 연결 상태가 보이면 자동으로 전환합니다."
			/>
		);
	}

	if (state.kind === "connected") {
		return (
			<StatePanel
				title={copy.onboarding.connected}
				body="머신이 연결되었습니다."
			/>
		);
	}

	if (state.kind === "expired") {
		return (
			<StatePanel
				title={copy.onboarding.expired}
				body={`만료 시간: ${state.expiresAt}`}
				action={copy.onboarding.retry}
				onAction={() => {
					void createInvite();
				}}
			/>
		);
	}

	return (
		<StatePanel
			title="등록 오류"
			body={state.message}
			action={copy.onboarding.retry}
			onAction={() => {
				void createInvite();
			}}
		/>
	);
}
