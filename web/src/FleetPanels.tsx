import { Filter, MoreVertical, Search } from "lucide-react";
import type { ReactElement } from "react";
import { copy } from "./copy";
import { parseOs, parseStatus } from "./filterParsers";
import type { Machine, MachineStatus, ResourceRow, SyncState } from "./types";

const statusLabels: Readonly<Record<MachineStatus, string>> = {
	healthy: "Healthy",
	drifted: "Drifted",
	pending: "Pending",
	offline: "Offline",
	blocked: "Blocked",
};

const syncLabels: Readonly<Record<SyncState, string>> = {
	applied: "Applied",
	pending: "Pending",
	drifted: "Drifted",
	blocked: "Blocked",
	rotating: "Rotating",
	na: "N/A",
};

export function MetricStrip({
	healthyCount,
	driftedCount,
	pendingCount,
	machineCount,
	onlineCount,
}: {
	readonly healthyCount: number;
	readonly driftedCount: number;
	readonly pendingCount: number;
	readonly machineCount: number;
	readonly onlineCount: number;
}): ReactElement {
	const metrics = [
		[
			copy.dashboard.metrics.machines,
			machineCount.toString(),
			`${onlineCount} online`,
		],
		[copy.dashboard.metrics.healthy, healthyCount.toString(), "80%"],
		[copy.dashboard.metrics.drifted, driftedCount.toString(), "needs review"],
		[
			copy.dashboard.metrics.pendingChanges,
			pendingCount.toString(),
			"2 resources",
		],
		[copy.dashboard.metrics.lastReconcile, "2m ago", "avg. across fleet"],
	] as const;
	return (
		<section className="metric-strip">
			{metrics.map(([label, value, note]) => (
				<div className="metric" key={label}>
					<span>{label}</span>
					<strong>{value}</strong>
					<small>{note}</small>
				</div>
			))}
		</section>
	);
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
	selectedMachine,
}: {
	readonly resources: readonly ResourceRow[];
	readonly selectedMachine: Machine;
}): ReactElement {
	return (
		<section className="ledger">
			<header>
				<div>
					<p className="eyebrow">Desired vs live</p>
					<h2>base + darwin + work</h2>
				</div>
				<button className="secondary-button" type="button">
					View history
				</button>
			</header>
			<div className="ledger-grid ledger-head">
				<span>Resource</span>
				<span>Desired</span>
				<span>Selected machine</span>
				<span>mac-studio</span>
				<span>homelab-node</span>
			</div>
			{resources.map((resource) => (
				<div
					className="ledger-grid ledger-row"
					key={`${resource.group}-${resource.name}`}
				>
					<span>
						<small>{resource.group}</small>
						<b>{resource.name}</b>
					</span>
					<span>{resource.desired}</span>
					<SyncBadge state={resource.states[selectedMachine.id] ?? "na"} />
					<SyncBadge state={resource.states["mac-studio"] ?? "na"} />
					<SyncBadge state={resource.states["homelab-node"] ?? "na"} />
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

function SyncBadge({ state }: { readonly state: SyncState }): ReactElement {
	return <span className={`sync ${state}`}>{syncLabels[state]}</span>;
}
