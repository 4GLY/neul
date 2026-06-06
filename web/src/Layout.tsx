import { Bell, BookOpen, ExternalLink, Lock, Search } from "lucide-react";
import type { ReactElement } from "react";
import { navGroups, securityItem } from "./data";

export function Sidebar(): ReactElement {
	return (
		<aside className="sidebar">
			<div className="brand">
				<div className="brand-mark">n</div>
				<span>neul</span>
			</div>
			<nav className="nav">
				{navGroups.map((group) => (
					<div className="nav-group" key={group.title}>
						<p>{group.title}</p>
						{group.items.map((item) => {
							const Icon = item.icon;
							return (
								<button
									className={
										item.label === "개요" ? "nav-item active" : "nav-item"
									}
									type="button"
									key={item.label}
								>
									<Icon size={16} />
									{item.label}
								</button>
							);
						})}
					</div>
				))}
			</nav>
			<div className="sidebar-card">
				<strong>neul</strong>
				<span>v0.5.2</span>
				<p>
					<span className="dot success" /> Up to date
				</p>
				<p>
					Control plane <b>self-hosted</b>
				</p>
				<p>
					Encryption <Lock size={13} /> E2E enabled
				</p>
				<a
					href="https://github.com/hoon-ch/neul"
					target="_blank"
					rel="noreferrer"
				>
					View docs <ExternalLink size={13} />
				</a>
			</div>
		</aside>
	);
}

export function Topbar({
	runState,
}: {
	readonly runState: "idle" | "running";
}): ReactElement {
	const SecurityIcon = securityItem.icon;
	return (
		<header className="topbar">
			<div className="search">
				<Search size={16} />
				<input
					aria-label="Search machines, packages, and dotfiles"
					placeholder="Search machines, packages, dotfiles..."
				/>
				<kbd>⌘K</kbd>
			</div>
			<div className="topbar-right">
				<span className="pill">
					<span className="dot success" />{" "}
					{runState === "running" ? "Reconcile running" : "All systems nominal"}
				</span>
				<span className="pill">
					Environment <b>self-hosted</b>
				</span>
				<span className="pill">
					<SecurityIcon size={14} /> {securityItem.label}
				</span>
				<BookOpen size={18} />
				<Bell size={18} />
				<div className="avatar">A</div>
			</div>
		</header>
	);
}
