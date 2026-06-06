import type { ReactElement } from "react";

export function StatePanel({
	title,
	body,
	action,
	onAction,
}: {
	readonly title: string;
	readonly body: string;
	readonly action?: string;
	readonly onAction?: () => void;
}): ReactElement {
	return (
		<section className="state-panel">
			<h2>{title}</h2>
			<p>{body}</p>
			{action === undefined ? null : (
				<button className="primary-button" type="button" onClick={onAction}>
					{action}
				</button>
			)}
		</section>
	);
}
