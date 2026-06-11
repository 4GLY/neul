import { Filter, MoreVertical, Search } from "lucide-react";
import type { ReactElement } from "react";
import type { DashboardMetrics } from "./api";
import { copy } from "./copy";
import { parseOs, parseStatus } from "./filterParsers";
import type { Machine, MachineStatus, ResourceRow } from "./types";

const statusLabels: Readonly<Record<MachineStatus, string>> = {
	healthy: "Healthy",
	drifted: "Drifted",
	pending: "Pending",
	offline: "Offline",
	blocked: "Blocked",
	unknown: "Unknown",
};

export function MetricStrip({
	metrics,
	latestReconcile,
}: {
	readonly metrics: DashboardMetrics;
	readonly latestReconcile: string;
}): ReactElement {
	const onlineCount =
		metrics.healthy + metrics.drifted + metrics.pending + metrics.blocked;
	const healthyRatio =
		metrics.total === 0
			? "0%"
			: `${Math.round((metrics.healthy / metrics.total) * 100)}%`;
	const metricCards = [
		[
			copy.dashboard.metrics.machines,
			metrics.total.toString(),
			`${onlineCount} online`,
		],
		[copy.dashboard.metrics.healthy, metrics.healthy.toString(), healthyRatio],
		[
			copy.dashboard.metrics.drifted,
			metrics.drifted.toString(),
			attentionNote(metrics),
		],
		[
			copy.dashboard.metrics.pendingChanges,
			metrics.pending.toString(),
			"awaiting apply",
		],
		[copy.dashboard.metrics.lastReconcile, latestReconcile, "latest report"],
	] as const;
	return (
		<section className="metric-strip">
			{metricCards.map(([label, value, note]) => (
				<div className="metric" key={label}>
					<span>{label}</span>
					<strong>{value}</strong>
					<small>{note}</small>
				</div>
			))}
		</section>
	);
}

function attentionNote(metrics: DashboardMetrics): string {
	return `${metrics.blocked} blocked · ${metrics.offline} offline · ${metrics.unknown} awaiting report`;
}

type MachineFiltersProps = {
	readonly statusFilter: "all" | MachineStatus;
	readonly osFilter: "all" | Machine["os"];
	readonly onStatusChange: (status: "all" | MachineStatus) => void;
	readonly onOsChange: (os: "all" | Machine["os"]) => void;
};

export function MachineFilters({
	statusFilter,
	osFilter,
	onStatusChange,
	onOsChange,
}: MachineFiltersProps): ReactElement {
	return (
		<section className="filters">
			<div className="filter-search">
				<Search size={15} />
				<input
					aria-label={copy.filters.searchMachines}
					placeholder="머신 검색..."
				/>
			</div>
			<select
				value={statusFilter}
				onChange={(event) => onStatusChange(parseStatus(event.target.value))}
			>
				<option value="all">{copy.filters.allStatus}</option>
				<option value="healthy">Healthy</option>
				<option value="drifted">Drifted</option>
				<option value="pending">Pending</option>
				<option value="offline">Offline</option>
				<option value="blocked">Blocked</option>
				<option value="unknown">Unknown</option>
			</select>
			<select
				value={osFilter}
				onChange={(event) => onOsChange(parseOs(event.target.value))}
			>
				<option value="all">{copy.filters.allOs}</option>
				<option value="macOS">macOS</option>
				<option value="Linux">Linux</option>
			</select>
			<button className="icon-button" type="button">
				<Filter size={15} /> {copy.filters.filters}
			</button>
		</section>
	);
}

export function MachineTable({
	machines: rows,
	selectedMachineId,
	onSelect,
}: {
	readonly machines: readonly Machine[];
	readonly selectedMachineId: string;
	readonly onSelect: (id: string) => void;
}): ReactElement {
	return (
		<section className="table-card">
			<div className="machine-grid table-head">
				<span>머신</span>
				<span>상태</span>
				<span>desired state</span>
				<span>drift</span>
				<span>최근 reconcile</span>
				<span>agent</span>
				<span />
			</div>
			{rows.map((machine) => (
				<button
					className={
						machine.id === selectedMachineId
							? "machine-grid row selected"
							: "machine-grid row"
					}
					type="button"
					key={machine.id}
					onClick={() => onSelect(machine.id)}
				>
					<span className="machine-name">
						<span className="device">{machine.os === "macOS" ? "⌘" : "λ"}</span>
						<b>{machine.name}</b>
						<small>
							{machine.os} {machine.version} ({machine.arch})
						</small>
						<em>{machine.tag}</em>
					</span>
					<StatusCell status={machine.status} note={machine.lastSeen} />
					<span>
						<b>{machine.desiredState}</b>
						<small>{machine.note}</small>
					</span>
					<span
						className={machine.driftCount > 0 ? "warning-text" : "success-text"}
					>
						<b>{machine.driftCount}</b>
						<small>{machine.driftCount > 0 ? "Drifted" : "No drift"}</small>
					</span>
					<span>
						<b>{machine.lastReconcile}</b>
						<small>progress {machine.progress}</small>
					</span>
					<span>
						<b>{machine.agent}</b>
						<small>
							<span
								className={
									machine.status === "offline" ? "dot danger" : "dot success"
								}
							/>{" "}
							{machine.status === "offline" ? "Reconnecting" : "Connected"}
						</small>
					</span>
					<MoreVertical size={16} />
				</button>
			))}
		</section>
	);
}

export function DesiredLivePreview({
	resources,
}: {
	readonly resources: readonly ResourceRow[];
}): ReactElement {
	return (
		<section className="ledger">
			<header>
				<div>
					<p className="eyebrow">Desired resources</p>
					<h2>base desired state</h2>
				</div>
				<button className="secondary-button" type="button">
					View history
				</button>
			</header>
			<div className="ledger-grid ledger-head">
				<span>Resource</span>
				<span>Desired</span>
			</div>
			{resources.map((resource) => (
				<div className="ledger-grid ledger-row" key={resource.id}>
					<span>
						<small>{resource.group}</small>
						<b>{resource.name}</b>
					</span>
					<span>{resource.desired}</span>
				</div>
			))}
		</section>
	);
}

function StatusCell({
	status,
	note,
}: {
	readonly status: MachineStatus;
	readonly note: string;
}): ReactElement {
	return (
		<span>
			<b className={`status ${status}`}>{statusLabels[status]}</b>
			<small>{note}</small>
		</span>
	);
}
