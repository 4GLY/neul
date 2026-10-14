import { ALERT_RULE_LABELS, type Alert } from "../shared/types";
import {
	Empty,
	ErrorNotice,
	Loading,
	PageHeading,
	relativeTime,
	useResource,
} from "./ui";

function formatTime(value: string) {
	return new Date(value).toLocaleString("ko-KR");
}

function AlertTable({
	alerts,
	resolved,
}: {
	alerts: Alert[];
	resolved: boolean;
}) {
	return (
		<div className="table-scroll inventory-scroll">
			<table>
				<thead>
					<tr>
						<th scope="col">머신</th>
						<th scope="col">규칙</th>
						<th scope="col">내용</th>
						<th scope="col">{resolved ? "발생 · 해결" : "발생"}</th>
					</tr>
				</thead>
				<tbody>
					{alerts.map((alert) => (
						<tr key={alert.id}>
							<th scope="row" className="row-key nowrap">
								<a href={`#/machines/${alert.machineId}`}>
									{alert.machineName}
								</a>
							</th>
							<td className="nowrap">
								<span className={`alert-rule ${alert.rule}`}>
									{ALERT_RULE_LABELS[alert.rule]}
								</span>
							</td>
							<td className="alert-message">{alert.message}</td>
							<td className="nowrap muted">
								<time
									dateTime={alert.openedAt}
									title={formatTime(alert.openedAt)}
								>
									{resolved
										? formatTime(alert.openedAt)
										: relativeTime(alert.openedAt)}
								</time>
								{resolved && alert.resolvedAt !== alert.openedAt ? (
									<>
										{" → "}
										<time dateTime={alert.resolvedAt ?? undefined}>
											{alert.resolvedAt ? formatTime(alert.resolvedAt) : ""}
										</time>
									</>
								) : null}
							</td>
						</tr>
					))}
				</tbody>
			</table>
		</div>
	);
}

export function Alerts() {
	const { data, error, loading, refresh } = useResource<Alert[]>(
		"/api/alerts?state=all",
		30_000,
	);
	const open = data?.filter((a) => a.resolvedAt === null) ?? [];
	const past = data?.filter((a) => a.resolvedAt !== null) ?? [];
	return (
		<>
			<PageHeading
				eyebrow="ALERTS"
				title="알림"
				description="규칙에 걸린 문제는 해결될 때까지 한 번만 푸시합니다. 규칙과 푸시 구독은 설정에서 바꿉니다."
			>
				<a className="button" href="#/settings">
					규칙 · 푸시 설정
				</a>
			</PageHeading>
			<ErrorNotice message={error} retry={() => void refresh()} />
			{loading ? (
				<Loading />
			) : (
				<>
					<section className="panel" aria-labelledby="open-alerts-title">
						<div className="section-heading">
							<div className="row-title">
								<h2 id="open-alerts-title">열린 알림</h2>
								<span className="count-label mono">{open.length}</span>
							</div>
							<span className="muted small">
								조건이 풀리면 자동으로 닫힙니다.
							</span>
						</div>
						{open.length ? (
							<AlertTable alerts={open} resolved={false} />
						) : (
							<Empty title="열린 알림이 없습니다." />
						)}
					</section>
					<section className="panel" aria-labelledby="past-alerts-title">
						<div className="section-heading">
							<h2 id="past-alerts-title">지난 알림</h2>
							<span className="muted small">최근 {past.length}건</span>
						</div>
						{past.length ? (
							<AlertTable alerts={past} resolved />
						) : (
							<Empty title="지난 알림이 없습니다." />
						)}
					</section>
				</>
			)}
		</>
	);
}
