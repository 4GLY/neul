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
import type { NavItem } from "./types";

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

export const securityItem = {
	icon: ShieldCheck,
	label: "E2E enabled",
} as const;
