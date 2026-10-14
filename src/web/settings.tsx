import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type { AlertRules, Settings as SettingsData } from "../shared/types";
import {
	api,
	ErrorNotice,
	errorMessage,
	Loading,
	PageHeading,
	Spinner,
	useResource,
} from "./ui";

export function Settings() {
	const { data, error, loading, refresh } =
		useResource<SettingsData>("/api/settings");
	return (
		<>
			<PageHeading
				eyebrow="HUB CONFIGURATION"
				title="허브 설정"
				description="추적할 파일, 자동 수집 주기, 알림 규칙과 푸시 알림을 관리합니다."
			/>
			<ErrorNotice message={error} retry={() => void refresh()} />
			{loading ? <Loading /> : data ? <SettingsForm initial={data} /> : null}
			<div className="settings-form">
				<PushSettings />
			</div>
		</>
	);
}

interface Threshold {
	on: boolean;
	value: string;
}

function threshold(value: number | null, fallback: number): Threshold {
	return { on: value !== null, value: String(value ?? fallback) };
}

function ThresholdField({
	label,
	unit,
	min,
	max,
	state,
	onChange,
}: {
	label: string;
	unit: string;
	min: number;
	max: number;
	state: Threshold;
	onChange: (next: Threshold) => void;
}) {
	return (
		<div className="threshold-row">
			<label className="checkbox-label">
				<input
					type="checkbox"
					checked={state.on}
					onChange={(event) => onChange({ ...state, on: event.target.checked })}
				/>
				{label}
			</label>
			<div className="interval-input">
				<input
					type="number"
					min={min}
					max={max}
					step={1}
					required={state.on}
					disabled={!state.on}
					aria-label={`${label} 기준`}
					value={state.value}
					onChange={(event) =>
						onChange({ ...state, value: event.target.value })
					}
				/>
				<span>{unit}</span>
			</div>
		</div>
	);
}

function SettingsForm({ initial }: { initial: SettingsData }) {
	const [paths, setPaths] = useState(initial.trackedPaths.join("\n"));
	const [interval, setInterval] = useState(String(initial.collectIntervalMin));
	const rules = initial.alertRules;
	const [unreachable, setUnreachable] = useState(
		threshold(rules.unreachableMin, 30),
	);
	const [disk, setDisk] = useState(threshold(rules.diskPercent, 90));
	const [updates, setUpdates] = useState(threshold(rules.updatesCount, 20));
	const [trackedFiles, setTrackedFiles] = useState(rules.trackedFiles);
	const [repos, setRepos] = useState(rules.repos.join("\n"));
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const [saved, setSaved] = useState(false);
	const lines = (text: string) =>
		text
			.split("\n")
			.map((line) => line.trim())
			.filter(Boolean);
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setBusy(true);
		setError("");
		setSaved(false);
		const value = (t: Threshold) => (t.on ? Number(t.value) : null);
		const alertRules: AlertRules = {
			unreachableMin: value(unreachable),
			diskPercent: value(disk),
			updatesCount: value(updates),
			trackedFiles,
			repos: lines(repos),
		};
		try {
			const result = await api<SettingsData>("/api/settings", "PUT", {
				trackedPaths: lines(paths),
				collectIntervalMin: Number(interval),
				alertRules,
			});
			setPaths(result.trackedPaths.join("\n"));
			setInterval(String(result.collectIntervalMin));
			setRepos(result.alertRules.repos.join("\n"));
			setSaved(true);
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}
	return (
		<form className="settings-form" onSubmit={(event) => void submit(event)}>
			<fieldset disabled={busy}>
				<section className="panel settings-section">
					<div>
						<h2>추적 경로</h2>
						<p className="muted">
							파일의 종류, 크기, 링크 대상과 SHA-256 해시를 수집합니다. 파일
							내용은 수집하지 않습니다.
						</p>
					</div>
					<label>
						<span>경로 패턴 · 한 줄에 하나</span>
						<textarea
							name="trackedPaths"
							rows={12}
							className="mono"
							value={paths}
							onChange={(event) => {
								setPaths(event.target.value);
								setSaved(false);
							}}
							aria-describedby="paths-help"
							placeholder={
								".zshrc\n.config/mise/config.toml\n.ssh/config.d/*.conf"
							}
						/>
						<span id="paths-help" className="muted small">
							$HOME 기준 상대 경로입니다. glob 패턴을 사용할 수 있으며, 매치되지
							않는 패턴은 missing으로 기록됩니다.
						</span>
					</label>
				</section>
				<section className="panel settings-section">
					<div>
						<h2>수집 주기</h2>
						<p className="muted">
							활성화된 머신을 자동 수집하는 간격입니다. 수동 수집은 언제든
							가능합니다.
						</p>
					</div>
					<label>
						자동 수집 간격
						<div className="interval-input">
							<input
								name="collectIntervalMin"
								type="number"
								min={1}
								step={1}
								required
								value={interval}
								onChange={(event) => {
									setInterval(event.target.value);
									setSaved(false);
								}}
							/>
							<span>분</span>
						</div>
					</label>
				</section>
				<section className="panel settings-section">
					<div>
						<h2>알림 규칙</h2>
						<p className="muted">
							수집 결과로 판정합니다. 문제가 생기면 한 번 푸시하고, 조건이
							풀리면 알림을 닫습니다. 끈 규칙의 열린 알림은 닫힙니다.
						</p>
					</div>
					<div className="rule-fields" onChangeCapture={() => setSaved(false)}>
						<ThresholdField
							label="연결 끊김"
							unit="분 넘게 수집 실패"
							min={1}
							max={10080}
							state={unreachable}
							onChange={setUnreachable}
						/>
						<ThresholdField
							label="디스크 사용률"
							unit="% 이상"
							min={1}
							max={100}
							state={disk}
							onChange={setDisk}
						/>
						<ThresholdField
							label="업데이트 대기"
							unit="개 이상 (brew + pacman)"
							min={1}
							max={10000}
							state={updates}
							onChange={setUpdates}
						/>
						<label className="checkbox-label">
							<input
								type="checkbox"
								checked={trackedFiles}
								onChange={(event) => setTrackedFiles(event.target.checked)}
							/>
							추적 파일 내용이 바뀌면 알림
						</label>
						<label>
							<span>감시할 git 저장소 · 한 줄에 하나</span>
							<textarea
								rows={4}
								className="mono"
								value={repos}
								onChange={(event) => setRepos(event.target.value)}
								aria-describedby="repos-help"
								placeholder={"repos/dotfiles\nrepos/neul"}
							/>
							<span id="repos-help" className="muted small">
								$HOME 기준 경로입니다. 커밋 안 한 변경이나 push 안 한 커밋이
								있으면 알립니다. 저장소가 없는 머신은 건너뜁니다.
							</span>
						</label>
					</div>
				</section>
			</fieldset>
			<ErrorNotice message={error} />
			<div className="form-footer">
				<p className="muted small">
					변경한 경로와 규칙은 다음 수집부터 적용됩니다.
				</p>
				<div className="actions">
					{saved ? (
						<span className="success-text" role="status">
							설정을 저장했습니다.
						</span>
					) : null}
					<button type="submit" className="primary" disabled={busy}>
						{busy ? <Spinner /> : null}
						{busy ? "저장 중…" : "설정 저장"}
					</button>
				</div>
			</div>
		</form>
	);
}

function base64UrlToBytes(value: string): Uint8Array<ArrayBuffer> {
	const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
	const binary = atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, "="));
	return Uint8Array.from(binary, (c) => c.charCodeAt(0));
}

type PushState =
	| { kind: "checking" }
	| { kind: "unsupported"; reason: string }
	| { kind: "ready"; subscription: PushSubscription | null };

/** This device's Web Push subscription; the service worker is registered in main.tsx. */
function PushSettings() {
	const [state, setState] = useState<PushState>({ kind: "checking" });
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const [message, setMessage] = useState("");

	useEffect(() => {
		if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
			const ios = /iPhone|iPad/.test(navigator.userAgent);
			setState({
				kind: "unsupported",
				reason: ios
					? "iPhone·iPad는 Safari 공유 메뉴에서 “홈 화면에 추가”한 뒤, 홈 화면의 늘 앱에서 이 화면을 여세요."
					: "이 브라우저는 Web Push를 지원하지 않습니다.",
			});
			return;
		}
		navigator.serviceWorker.ready
			.then((registration) => registration.pushManager.getSubscription())
			.then((subscription) => setState({ kind: "ready", subscription }))
			.catch((cause) =>
				setState({ kind: "unsupported", reason: errorMessage(cause) }),
			);
	}, []);

	async function run(action: () => Promise<void>) {
		setBusy(true);
		setError("");
		setMessage("");
		try {
			await action();
		} catch (cause) {
			setError(errorMessage(cause));
		} finally {
			setBusy(false);
		}
	}

	const subscribe = () =>
		run(async () => {
			if ((await Notification.requestPermission()) !== "granted")
				throw new Error(
					"알림 권한이 거부되었습니다. 브라우저 설정에서 허용해 주세요.",
				);
			const { publicKey } = await api<{ publicKey: string }>("/api/push/key");
			const registration = await navigator.serviceWorker.ready;
			const subscription = await registration.pushManager.subscribe({
				userVisibleOnly: true,
				applicationServerKey: base64UrlToBytes(publicKey),
			});
			await api("/api/push/subscriptions", "POST", subscription.toJSON());
			setState({ kind: "ready", subscription });
			setMessage("이 기기에서 알림을 받습니다.");
		});

	const unsubscribe = (subscription: PushSubscription) =>
		run(async () => {
			await api("/api/push/subscriptions", "DELETE", {
				endpoint: subscription.endpoint,
			});
			await subscription.unsubscribe();
			setState({ kind: "ready", subscription: null });
			setMessage("이 기기의 알림을 껐습니다.");
		});

	const test = () =>
		run(async () => {
			const { sent, failed } = await api<{ sent: number; failed: number }>(
				"/api/push/test",
				"POST",
			);
			setMessage(
				`구독한 기기 ${sent}대로 보냈습니다${failed ? ` (실패 ${failed})` : ""}.`,
			);
		});

	return (
		<section className="panel settings-section push-settings">
			<div>
				<h2>푸시 알림</h2>
				<p className="muted">
					허브가 Web Push로 이 기기에 직접 알립니다. 기기마다 따로 켭니다.
				</p>
			</div>
			<div className="rule-fields">
				{state.kind === "checking" ? (
					<p className="muted small" role="status">
						<Spinner /> 이 기기의 구독 상태를 확인하는 중…
					</p>
				) : state.kind === "unsupported" ? (
					<p className="muted">{state.reason}</p>
				) : (
					<>
						<p>
							이 기기:{" "}
							<strong className={state.subscription ? "success-text" : ""}>
								{state.subscription ? "알림 받는 중" : "알림 꺼짐"}
							</strong>
						</p>
						<div className="actions">
							{state.subscription ? (
								<button
									type="button"
									onClick={() => {
										if (state.subscription)
											void unsubscribe(state.subscription);
									}}
									disabled={busy}
								>
									이 기기 알림 끄기
								</button>
							) : (
								<button
									type="button"
									className="primary"
									onClick={() => void subscribe()}
									disabled={busy}
								>
									{busy ? <Spinner /> : null}이 기기에서 알림 받기
								</button>
							)}
							<button type="button" onClick={() => void test()} disabled={busy}>
								테스트 알림 보내기
							</button>
						</div>
					</>
				)}
				{message ? (
					<p className="success-text small" role="status">
						{message}
					</p>
				) : null}
				<ErrorNotice message={error} />
			</div>
		</section>
	);
}
