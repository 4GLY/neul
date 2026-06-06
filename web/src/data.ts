import {
	Activity,
	CalendarDays,
	FileCode2,
	Gauge,
	GitCompare,
	KeyRound,
	Layers3,
	Monitor,
	Package,
	RotateCw,
	Settings,
	ShieldCheck,
	SlidersHorizontal,
} from "lucide-react";
import type {
	Activity as ActivityItem,
	Machine,
	NavItem,
	ResourceRow,
} from "./types";

export const navGroups: readonly {
	readonly title: string;
	readonly items: readonly NavItem[];
}[] = [
	{
		title: "머신",
		items: [
			{ label: "개요", icon: Gauge },
			{ label: "머신 목록", icon: Monitor },
			{ label: "프로필", icon: Layers3 },
			{ label: "세그먼트", icon: SlidersHorizontal },
		],
	},
	{
		title: "상태",
		items: [
			{ label: "패키지", icon: Package },
			{ label: "dotfile", icon: FileCode2 },
			{ label: "secret", icon: KeyRound },
		],
	},
	{
		title: "자동화",
		items: [
			{ label: "Reconcile", icon: RotateCw },
			{ label: "Drift", icon: GitCompare },
			{ label: "스케줄", icon: CalendarDays },
		],
	},
	{
		title: "시스템",
		items: [
			{ label: "감사 로그", icon: Activity },
			{ label: "설정", icon: Settings },
		],
	},
] as const;

export const machines: readonly Machine[] = [
	{
		id: "mac-studio",
		name: "mac-studio",
		os: "macOS",
		version: "14.4",
		arch: "arm64",
		tag: "work",
		agent: "v0.5.2",
		status: "healthy",
		desiredState: "In sync",
		driftCount: 0,
		lastReconcile: "2m ago",
		lastSeen: "just now",
		progress: "5 / 5",
		note: "All good",
	},
	{
		id: "work-macbook",
		name: "work-macbook",
		os: "macOS",
		version: "14.3",
		arch: "arm64",
		tag: "mobile",
		agent: "v0.5.2",
		status: "drifted",
		desiredState: "Drift detected",
		driftCount: 3,
		lastReconcile: "6m ago",
		lastSeen: "just now",
		progress: "2 / 5",
		note: "2 packages, 1 dotfile",
	},
	{
		id: "linux-vm",
		name: "linux-vm",
		os: "Linux",
		version: "Ubuntu 22.04",
		arch: "x86_64",
		tag: "lab",
		agent: "v0.5.1",
		status: "pending",
		desiredState: "Applying",
		driftCount: 1,
		lastReconcile: "12m ago",
		lastSeen: "30s ago",
		progress: "3 / 5",
		note: "Dotfiles running",
	},
	{
		id: "homelab-node",
		name: "homelab-node",
		os: "Linux",
		version: "Debian 12",
		arch: "x86_64",
		tag: "home",
		agent: "v0.5.2",
		status: "offline",
		desiredState: "Unknown",
		driftCount: 0,
		lastReconcile: "18m ago",
		lastSeen: "2m ago",
		progress: "0 / 5",
		note: "Agent reconnecting",
	},
] as const;

export const activities: readonly ActivityItem[] = [
	{
		id: "a1",
		tone: "success",
		title: "Reconcile succeeded on mac-studio",
		time: "2m ago",
		details: ["1 package installed", "1 dotfile updated"],
		scope: "base, work",
	},
	{
		id: "a2",
		tone: "warning",
		title: "Drift detected on work-macbook",
		time: "6m ago",
		details: ["kubectl behind desired version", "~/.zshrc hash differs"],
		scope: "mobile",
	},
	{
		id: "a3",
		tone: "info",
		title: "Secret rotated on homelab-node",
		time: "18m ago",
		details: ["SSH_KEY recipient updated"],
		scope: "production",
	},
	{
		id: "a4",
		tone: "success",
		title: "Dotfile applied on linux-vm",
		time: "27m ago",
		details: ["~/.gitconfig linked"],
		scope: "base",
	},
] as const;

export const resources: readonly ResourceRow[] = [
	{
		group: "패키지",
		name: "kubectl",
		desired: "1.31.0",
		states: {
			"mac-studio": "applied",
			"work-macbook": "pending",
			"linux-vm": "applied",
			"homelab-node": "drifted",
		},
	},
	{
		group: "패키지",
		name: "mise node",
		desired: "22.2.0",
		states: {
			"mac-studio": "applied",
			"work-macbook": "applied",
			"linux-vm": "pending",
			"homelab-node": "pending",
		},
	},
	{
		group: "dotfile",
		name: "~/.zshrc",
		desired: "a1b2c3d",
		states: {
			"mac-studio": "applied",
			"work-macbook": "drifted",
			"linux-vm": "applied",
			"homelab-node": "applied",
		},
	},
	{
		group: "dotfile",
		name: "~/.gitconfig",
		desired: "c3d4e5f",
		states: {
			"mac-studio": "applied",
			"work-macbook": "applied",
			"linux-vm": "applied",
			"homelab-node": "applied",
		},
	},
	{
		group: "secret",
		name: "GITHUB_TOKEN",
		desired: "repo, workflow",
		states: {
			"mac-studio": "applied",
			"work-macbook": "applied",
			"linux-vm": "applied",
			"homelab-node": "rotating",
		},
	},
] as const;

export const securityItem = {
	icon: ShieldCheck,
	label: "E2E enabled",
} as const;
