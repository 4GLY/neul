import { CheckCircle2, CircleAlert, Clock3, XCircle } from "lucide-react";
import type { ReactElement } from "react";
import type { MachineEvent } from "./api";
import { copy } from "./copy";
import type { Activity, Machine, MachineStatus } from "./types";

const statusLabels: Readonly<Record<MachineStatus, string>> = {
	healthy: "Healthy",
	drifted: "Drifted",
	pending: "Pending",
	offline: "Offline",
	blocked: "Blocked",
};

export function MachineInspector({
	machine,
	events,
	onRepairDrift,
	onOpenLogs,
}: {
	readonly machine: Machine;
	readonly events: readonly MachineEvent[];
	readonly onRepairDrift: () => void;
	readonly onOpenLogs: () => void;
}): ReactElement {
	return (
		<section className="inspector">
			<header>
				<span className="device large">
					{machine.os === "macOS" ? "⌘" : "λ"}
				</span>
				<div>
					<h2>{machine.name}</h2>
					<p>
						{machine.os} {machine.version} ({machine.arch})
					</p>
				</div>
			</header>
			<div className="tabs">
				<button className="active" type="button">
					{copy.inspector.tabs.status}
				</button>
				<button type="button">{copy.inspector.tabs.changes}</button>
				<button type="button">{copy.inspector.tabs.config}</button>
				<button type="button">{copy.inspector.tabs.logs}</button>
			</div>
			<div
				className={machine.status === "drifted" ? "notice warning" : "notice"}
			>
				{machine.status === "drifted" ? (
					<CircleAlert size={18} />
				) : (
					<CheckCircle2 size={18} />
				)}
				<span>
					<b>{statusLabels[machine.status]}</b>
					{machine.note}
				</span>
			</div>
			<div className="progress-card">
				<p>
					<span className="step">2</span> Package adapter{" "}
					<b>{machine.progress}</b>
				</p>
				<div className="progress-bar">
					<span
						style={{ width: machine.status === "healthy" ? "100%" : "46%" }}
					/>
				</div>
			</div>
			<div className="inspector-actions">
				<button
					className="primary-button"
					type="button"
					onClick={onRepairDrift}
				>
					{copy.inspector.repairDrift}
				</button>
				<button className="secondary-button" type="button">
					{copy.inspector.viewDiff}
				</button>
				<button className="secondary-button" type="button" onClick={onOpenLogs}>
					{copy.inspector.openLogs}
				</button>
			</div>
			{events.length === 0 ? null : (
				<div className="event-list">
					<h3>최근 이벤트</h3>
					{events.map((event) => (
						<p key={event.id}>
							<b>{event.status}</b>
							{event.message}
						</p>
					))}
				</div>
			)}
		</section>
	);
}

export function ActivityFeed({
	activities,
}: {
	readonly activities: readonly Activity[];
}): ReactElement {
	return (
		<section className="activity">
			<header>
				<h2>Activity</h2>
				<a href="#top">View all</a>
			</header>
			{activities.map((item) => (
				<ActivityRow item={item} key={item.id} />
			))}
		</section>
	);
}

function ActivityRow({ item }: { readonly item: Activity }): ReactElement {
	const Icon =
		item.tone === "danger"
			? XCircle
			: item.tone === "warning"
				? CircleAlert
				: item.tone === "info"
					? Clock3
					: CheckCircle2;
	return (
		<article className="activity-row">
			<Icon className={item.tone} size={21} />
			<div>
				<h3>{item.title}</h3>
				<ul>
					{item.details.map((detail) => (
						<li key={detail}>{detail}</li>
					))}
				</ul>
				<span>{item.scope}</span>
			</div>
			<time>{item.time}</time>
		</article>
	);
}
