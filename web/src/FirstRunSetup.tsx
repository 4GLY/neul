import type { FormEvent, ReactElement } from "react";
import { useState } from "react";
import { createLocalSession, LocalSessionError } from "./api";

const defaultSetupMessage =
	"서버 콘솔에 한 번만 출력된 setup token을 입력하세요.";

export function FirstRunSetup({
	onSetupComplete,
}: {
	readonly onSetupComplete: () => Promise<void>;
}): ReactElement {
	const [setupToken, setSetupToken] = useState("");
	const [message, setMessage] = useState(defaultSetupMessage);
	const [submitting, setSubmitting] = useState(false);

	async function handleSubmit(
		event: FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault();
		setSubmitting(true);
		try {
			await createLocalSession(setupToken.trim());
			setSetupToken("");
			setMessage(defaultSetupMessage);
			await onSetupComplete();
		} catch (error) {
			if (error instanceof LocalSessionError) {
				setSetupToken("");
				setMessage(error.message);
				return;
			}
			throw error;
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<main className="shell">
			<section className="workspace setup-workspace">
				<div className="content-grid setup-grid">
					<section className="main-column">
						<section className="state-panel" aria-label="첫 실행 설정">
							<p className="eyebrow">owner setup</p>
							<h1>첫 실행 설정</h1>
							<p>{message}</p>
							<form className="setup-form" onSubmit={handleSubmit}>
								<label>
									setup token
									<input
										aria-label="setup token"
										autoComplete="one-time-code"
										name="setup-token"
										onChange={(event) => setSetupToken(event.target.value)}
										type="password"
										value={setupToken}
									/>
								</label>
								<button
									className="primary-button"
									disabled={submitting}
									type="submit"
								>
									{submitting ? "확인 중" : "설정 완료"}
								</button>
							</form>
						</section>
					</section>
				</div>
			</section>
		</main>
	);
}
