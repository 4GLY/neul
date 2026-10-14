import type { FormEvent } from "react";
import { useState } from "react";
import type { Settings as SettingsData } from "../shared/types";
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
				description="추적할 파일과 자동 수집 주기를 관리합니다."
			/>
			<ErrorNotice message={error} retry={() => void refresh()} />
			{loading ? <Loading /> : data ? <SettingsForm initial={data} /> : null}
		</>
	);
}

function SettingsForm({ initial }: { initial: SettingsData }) {
	const [paths, setPaths] = useState(initial.trackedPaths.join("\n"));
	const [interval, setInterval] = useState(String(initial.collectIntervalMin));
	const [busy, setBusy] = useState(false);
	const [error, setError] = useState("");
	const [saved, setSaved] = useState(false);
	async function submit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setBusy(true);
		setError("");
		setSaved(false);
		try {
			const result = await api<SettingsData>("/api/settings", "PUT", {
				trackedPaths: paths
					.split("\n")
					.map((path) => path.trim())
					.filter(Boolean),
				collectIntervalMin: Number(interval),
			});
			setPaths(result.trackedPaths.join("\n"));
			setInterval(String(result.collectIntervalMin));
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
			</fieldset>
			<ErrorNotice message={error} />
			<div className="form-footer">
				<p className="muted small">변경한 경로는 다음 수집부터 적용됩니다.</p>
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
