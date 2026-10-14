import type { ReactNode } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Facts, MachineStatus } from "../shared/types";

export async function api<T>(
	path: string,
	method = "GET",
	body?: unknown,
	signal?: AbortSignal,
): Promise<T> {
	const response = await fetch(path, {
		method,
		signal,
		...(body === undefined
			? {}
			: {
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(body),
				}),
	});
	if (!response.ok) {
		const payload = (await response.json().catch(() => null)) as {
			error?: string;
		} | null;
		throw new Error(payload?.error ?? `요청 실패 (${response.status})`);
	}
	if (response.status === 204) return undefined as T;
	return response.json() as Promise<T>;
}

export function useResource<T>(path: string, interval = 0) {
	const [data, setData] = useState<T | null>(null);
	const [error, setError] = useState("");
	const [loading, setLoading] = useState(true);
	const controller = useRef<AbortController | null>(null);
	const refresh = useCallback(async () => {
		controller.current?.abort();
		const current = new AbortController();
		controller.current = current;
		try {
			const result = await api<T>(path, "GET", undefined, current.signal);
			if (!current.signal.aborted) {
				setData(result);
				setError("");
			}
		} catch (cause) {
			if (!current.signal.aborted) setError(errorMessage(cause));
		} finally {
			if (!current.signal.aborted) setLoading(false);
		}
	}, [path]);
	useEffect(() => {
		setData(null);
		setLoading(true);
		void refresh();
		const timer = interval
			? window.setInterval(() => void refresh(), interval)
			: null;
		return () => {
			controller.current?.abort();
			if (timer !== null) window.clearInterval(timer);
		};
	}, [refresh, interval]);
	return { data, error, loading, refresh };
}

export function errorMessage(cause: unknown) {
	return cause instanceof Error ? cause.message : "요청을 처리하지 못했습니다.";
}

export function ErrorNotice({
	message,
	retry,
}: {
	message: string;
	retry?: () => void;
}) {
	if (!message) return null;
	return (
		<div className="notice error" role="alert">
			<span>{message}</span>
			{retry ? (
				<button type="button" className="quiet" onClick={retry}>
					다시 시도
				</button>
			) : null}
		</div>
	);
}

export function Empty({
	title,
	children,
}: {
	title: string;
	children?: ReactNode;
}) {
	return (
		<div className="empty">
			<strong>{title}</strong>
			{children ? <div>{children}</div> : null}
		</div>
	);
}

export function Loading() {
	return (
		<div className="empty" role="status">
			<Spinner /> 데이터를 불러오는 중…
		</div>
	);
}

export function Spinner() {
	return <span className="spinner" aria-hidden="true" />;
}

const statusLabels: Record<MachineStatus, string> = {
	online: "온라인",
	unreachable: "연결 불가",
	error: "오류",
	unknown: "미수집",
};
export function Status({
	status,
	collecting = false,
}: {
	status: MachineStatus;
	collecting?: boolean;
}) {
	return (
		<span className={`status ${collecting ? "collecting" : status}`}>
			{collecting ? <Spinner /> : <span className="status-dot" />}
			{collecting ? "수집 중" : statusLabels[status]}
		</span>
	);
}

export function relativeTime(value: string | null) {
	if (!value) return "수집 기록 없음";
	const seconds = Math.max(0, (Date.now() - Date.parse(value)) / 1000);
	if (seconds < 60) return "방금 전";
	if (seconds < 3600) return `${Math.floor(seconds / 60)}분 전`;
	if (seconds < 86400) return `${Math.floor(seconds / 3600)}시간 전`;
	return `${Math.floor(seconds / 86400)}일 전`;
}
export function uptime(seconds: number | null | undefined) {
	if (seconds == null) return "—";
	const days = Math.floor(seconds / 86400);
	const hours = Math.floor((seconds % 86400) / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	return days
		? `${days}일 ${hours}시간`
		: hours
			? `${hours}시간 ${minutes}분`
			: `${minutes}분`;
}
export function bytes(value: number | null | undefined) {
	if (value == null) return "—";
	if (value < 1024) return `${value} B`;
	const units = ["KiB", "MiB", "GiB", "TiB"];
	const index = Math.min(
		Math.floor(Math.log(value) / Math.log(1024)) - 1,
		units.length - 1,
	);
	return `${(value / 1024 ** (index + 1)).toFixed(1)} ${units[index]}`;
}
export function memoryPercent(facts: Facts | null) {
	return facts?.memTotalB && facts.memAvailableB !== null
		? (1 - facts.memAvailableB / facts.memTotalB) * 100
		: null;
}
export function diskPercent(facts: Facts | null) {
	return facts?.disk?.totalB
		? (facts.disk.usedB / facts.disk.totalB) * 100
		: null;
}
export function Meter({
	label,
	value,
}: {
	label: string;
	value: number | null;
}) {
	const percent = value === null ? null : Math.max(0, Math.min(100, value));
	return (
		<div className="meter">
			<div className="meter-label">
				<span>{label}</span>
				<span className="mono">
					{percent === null ? "—" : `${percent.toFixed(0)}%`}
				</span>
			</div>
			<progress
				className={percent !== null && percent >= 90 ? "high" : ""}
				aria-label={label}
				max={100}
				value={percent ?? 0}
			/>
		</div>
	);
}

export function PageHeading({
	eyebrow,
	title,
	children,
	description,
}: {
	eyebrow: string;
	title: string;
	children?: ReactNode;
	description?: string;
}) {
	return (
		<div className="page-heading">
			<div>
				<p className="eyebrow">{eyebrow}</p>
				<h1>{title}</h1>
				{description ? (
					<p className="muted description">{description}</p>
				) : null}
			</div>
			<div className="actions">{children}</div>
		</div>
	);
}
